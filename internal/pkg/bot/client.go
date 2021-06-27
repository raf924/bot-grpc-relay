package bot

import (
	"context"
	"errors"
	"fmt"
	cnf "github.com/raf924/bot-grpc-relay/internal/pkg/config"
	"github.com/raf924/bot-grpc-relay/pkg/utils"
	"github.com/raf924/bot/pkg/queue"
	"github.com/raf924/bot/pkg/users"
	api "github.com/raf924/connector-api/pkg/gen"
	messages "github.com/raf924/connector-api/pkg/gen"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/emptypb"
	"gopkg.in/yaml.v2"
	"io"
	"log"
	"time"
)

func NewGrpcRelayClient(config interface{}, botExchange *queue.Exchange) *grpcRelayClient {
	data, err := yaml.Marshal(config)
	if err != nil {
		panic(err)
	}
	var conf cnf.GrpcClientConfig
	if err := yaml.Unmarshal(data, &conf); err != nil {
		panic(err)
	}
	return newGrpcRelayClient(conf, botExchange)
}

func newGrpcRelayClient(config cnf.GrpcClientConfig, botExchange *queue.Exchange) *grpcRelayClient {
	return &grpcRelayClient{config: config, botExchange: botExchange}
}

type streamReader func(stream grpc.ClientStream) (proto.Message, error)

type grpcRelayClient struct {
	config          cnf.GrpcClientConfig
	connectorClient api.ConnectorClient
	users           *users.UserList
	onUserJoin      func(user *messages.User, timestamp int64)
	onUserLeft      func(user *messages.User, timestamp int64)
	ctx             context.Context
	cancel          context.CancelFunc
	session         string
	botExchange     *queue.Exchange
}

func (g *grpcRelayClient) Done() <-chan struct{} {
	return g.ctx.Done()
}

func (g *grpcRelayClient) GetUsers() []*messages.User {
	return g.users.All()
}

func (g *grpcRelayClient) OnUserJoin(f func(user *messages.User, timestamp int64)) {
	g.onUserJoin = func(user *messages.User, timestamp int64) {
		if user == nil {
			return
		}
		g.users.Add(user)
		f(user, timestamp)
	}
}

func (g *grpcRelayClient) OnUserLeft(f func(user *messages.User, timestamp int64)) {
	g.onUserLeft = func(user *messages.User, timestamp int64) {
		g.users.Remove(user)
		f(user, timestamp)
	}
}

func (g *grpcRelayClient) readUserEvent(stream grpc.ClientStream) (proto.Message, error) {
	packet, err := stream.(messages.Connector_ReadUserEventsClient).Recv()
	if err != nil {
		return nil, err
	}
	switch packet.Event {
	case messages.UserEvent_JOINED:
		if g.onUserJoin == nil {
			break
		}
		g.onUserJoin(packet.User, packet.Timestamp.GetSeconds())
	case messages.UserEvent_LEFT:
		if g.onUserLeft == nil {
			break
		}
		g.onUserLeft(packet.User, packet.Timestamp.GetSeconds())
	}
	return packet, nil
}

func (g *grpcRelayClient) readMessage(stream grpc.ClientStream) (proto.Message, error) {
	message, err := stream.(messages.Connector_ReadMessagesClient).Recv()
	if err != nil {
		return nil, err
	}
	log.Println("[", message.GetTimestamp().AsTime().String(), "]", "-", message.GetUser().GetNick(), ":", message.GetMessage())
	return message, nil
}

func (g *grpcRelayClient) readCommand(stream grpc.ClientStream) (proto.Message, error) {
	packet, err := stream.(messages.Connector_ReadCommandsClient).Recv()
	if err != nil {
		return nil, err
	}
	log.Println("[", packet.GetTimestamp().AsTime().String(), "]", "-", packet.GetCommand(), "(", packet.GetArgString(), ")")
	return packet, nil
}

func (g *grpcRelayClient) relayStream(stream grpc.ClientStream, reader streamReader) error {
	for {
		packet, err := reader(stream)
		if err != nil {
			if errors.Is(err, io.EOF) {
				return nil
			}
			return err
		}
		err = g.relayToBot(packet)
		if err != nil {
			return err
		}
	}
}

func (g *grpcRelayClient) getSessionContext(ctx ...context.Context) context.Context {
	if len(ctx) == 0 {
		ctx = append(ctx, context.Background())
	}
	return metadata.AppendToOutgoingContext(ctx[0], "sessionid", g.session)
}

func (g *grpcRelayClient) connect(registration *messages.RegistrationPacket, conn *grpc.ClientConn) (*messages.User, error) {
	g.OnUserJoin(func(user *messages.User, timestamp int64) {})
	g.OnUserLeft(func(user *messages.User, timestamp int64) {})
	g.ctx, g.cancel = context.WithCancel(context.Background())
	g.connectorClient = api.NewConnectorClient(conn)
	if g.connectorClient == nil {
		return nil, errors.New("couldn't create client")
	}
	md := metadata.New(map[string]string{})
	_, err := g.connectorClient.Connect(context.Background(), &emptypb.Empty{}, grpc.Header(&md))
	if err != nil {
		return nil, fmt.Errorf("couldn't connect: %v", err)
	}
	g.session = md.Get("sessionid")[0]
	confirmation, err := g.connectorClient.Register(g.getSessionContext(), registration)
	if err != nil {
		return nil, fmt.Errorf("couldn't register: %v", err)
	}
	if confirmation == nil {
		return nil, errors.New("no confirmation from connectorClient")
	}
	g.users = users.NewUserList(confirmation.Users...)
	eventStream, err := g.connectorClient.ReadUserEvents(g.getSessionContext(), &emptypb.Empty{})
	if err != nil {
		return nil, err
	}
	go func() {
		err := g.relayStream(eventStream, g.readUserEvent)
		if !errors.Is(err, io.EOF) {
			g.cancel()
			return
		}
	}()
	messageStream, err := g.connectorClient.ReadMessages(g.getSessionContext(), &emptypb.Empty{})
	if err != nil {
		return nil, err
	}
	go func() {
		err := g.relayStream(messageStream, g.readMessage)
		if !errors.Is(err, io.EOF) {
			g.cancel()
			return
		}
	}()
	commandStream, err := g.connectorClient.ReadCommands(g.getSessionContext(g.ctx), &emptypb.Empty{})
	if err != nil {
		return nil, err
	}
	go func() {
		err := g.relayStream(commandStream, g.readCommand)
		if !errors.Is(err, io.EOF) {
			g.cancel()
			return
		}
	}()
	go func() {
		for {
			p, err := g.botExchange.Consume()
			if err != nil {
				panic(err)
			}
			err = g.send(p.(*messages.BotPacket))
			if err != nil {
				log.Println("Couldn't send packet to relay server:", p, "=>", err)
			}
		}
	}()
	return confirmation.BotUser, nil
}

func (g *grpcRelayClient) Connect(registration *messages.RegistrationPacket) (*messages.User, error) {
	var clientOption = grpc.WithInsecure()
	var err error
	if g.config.Tls.Enabled {
		log.Println("Using TLS Client Configuration")
		clientOption, err = utils.LoadMutualTLSClientConfig(g.config.Tls.Name, g.config.Tls.Ca, g.config.Tls.Cert, g.config.Tls.Key)
		if err != nil {
			return nil, err
		}
	}
	log.Println("Connecting")
	conn, err := grpc.Dial(fmt.Sprintf("%s:%d", g.config.Host, g.config.Port), clientOption, grpc.WithBlock())
	if err != nil {
		log.Println("Failed to connect")
		return nil, err
	}
	log.Println("Connected")
	return g.connect(registration, conn)
}

func (g *grpcRelayClient) relayToBot(message proto.Message) error {
	log.Println("relaying to bot", message)
	return g.botExchange.Produce(message)
}

func (g *grpcRelayClient) send(message *messages.BotPacket) error {
	ctx, _ := context.WithTimeout(context.Background(), 5*time.Second)
	_, err := g.connectorClient.SendMessage(g.getSessionContext(ctx), message)
	return err
}
