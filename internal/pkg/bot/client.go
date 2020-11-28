package bot

import (
	"context"
	"errors"
	"fmt"
	api "github.com/raf924/bot-grpc-relay/internal/api/grpc"
	"github.com/raf924/bot-grpc-relay/internal/pkg/utils"
	"github.com/raf924/bot/api/messages"
	"github.com/raf924/bot/pkg/relay"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/known/emptypb"
	"gopkg.in/yaml.v2"
	"io"
	"log"
	"time"
)

func init() {
	relay.RegisterConnectorRelay("grpc", newConnectorRelay)
}

func newConnectorRelay(config interface{}) relay.ConnectorRelay {
	data, err := yaml.Marshal(config)
	if err != nil {
		panic(err)
	}
	var conf grpcClientConfig
	if err := yaml.Unmarshal(data, &conf); err != nil {
		panic(err)
	}
	return &grpcConnectorRelay{config: conf}
}

func readStream(stream grpc.ClientStream, f func(message protoreflect.ProtoMessage) error) error {
	var err error
	var packet protoreflect.ProtoMessage
	for ; err == nil; err = stream.RecvMsg(packet) {
		err := f(packet)
		if err != nil {
			log.Println(err)
		}
	}
	return err
}

func readEventStream(stream grpc.ClientStream, f func(event *messages.UserPacket) error) error {
	var err error
	var packet messages.UserPacket
	for ; err == nil; err = stream.RecvMsg(&packet) {
		err := f(&packet)
		if err != nil {
			log.Println(err)
		}
	}
	return err
}

func readMessageStream(stream grpc.ClientStream, f func(message *messages.MessagePacket) error) error {
	var err error
	var packet messages.MessagePacket
	for ; err == nil; err = stream.RecvMsg(&packet) {
		err := f(&packet)
		if err != nil {
			log.Println(err)
		}
	}
	return err
}

func readCommandStream(stream grpc.ClientStream, f func(message *messages.CommandPacket) error) error {
	var err error
	var packet messages.CommandPacket
	for ; err == nil; err = stream.RecvMsg(&packet) {
		err := f(&packet)
		if err != nil {
			log.Println(err)
		}
	}
	return err
}

type grpcConnectorRelay struct {
	config          grpcClientConfig
	connector       api.ConnectorClient
	users           []*messages.User
	onUserJoin      func(user *messages.User, timestamp int64)
	onUserLeft      func(user *messages.User, timestamp int64)
	messageStream   grpc.ClientStream
	commandStream   grpc.ClientStream
	incomingChannel chan protoreflect.ProtoMessage
}

func (g *grpcConnectorRelay) GetUsers() []*messages.User {
	return g.users
}

func (g *grpcConnectorRelay) OnUserJoin(f func(user *messages.User, timestamp int64)) {
	g.onUserJoin = f
}

func (g *grpcConnectorRelay) OnUserLeft(f func(user *messages.User, timestamp int64)) {
	g.onUserLeft = f
}

func (g *grpcConnectorRelay) Connect(registration *messages.RegistrationPacket) (*messages.User, error) {
	var clientOption = grpc.WithInsecure()
	var err error
	if g.config.Tls.Enabled {
		clientOption, err = utils.LoadTLSClientConfig(g.config.Tls.Name, g.config.Tls.Ca, g.config.Tls.Cert, g.config.Tls.Key)
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
	g.connector = api.NewConnectorClient(conn)
	if g.connector == nil {
		return nil, errors.New("couldn't create client")
	}
	g.incomingChannel = make(chan protoreflect.ProtoMessage)
	confirmation, err := g.connector.Register(context.Background(), registration)
	if err != nil {
		return nil, err
	}
	if confirmation == nil {
		return nil, errors.New("no confirmation from connector")
	}
	g.users = confirmation.Users
	eventStream, err := g.connector.ReadUserEvents(context.Background(), &emptypb.Empty{})
	if err != nil {
		return nil, err
	}
	go func() {
		err := readEventStream(eventStream, func(event *messages.UserPacket) error {
			switch event.Event {
			case messages.UserEvent_JOINED:
				if g.onUserJoin == nil {
					return nil
				}
				g.onUserJoin(event.User, event.Timestamp.GetSeconds())
			case messages.UserEvent_LEFT:
				if g.onUserLeft == nil {
					return nil
				}
				g.onUserJoin(event.User, event.Timestamp.GetSeconds())
			}
			return nil
		})
		if !errors.Is(err, io.EOF) {
			panic(err)
		}
	}()
	g.messageStream, err = g.connector.ReadMessages(context.Background(), &emptypb.Empty{})
	if err != nil {
		return nil, err
	}
	go func() {
		err := readMessageStream(g.messageStream, func(message *messages.MessagePacket) error {
			if message.Timestamp == nil {
				return nil
			}
			log.Println(message.Timestamp.AsTime().String(), "-", message.User.Nick, ":", message.Message)
			g.incomingChannel <- message
			return nil
		})
		if !errors.Is(err, io.EOF) {
			if !errors.Is(err, grpc.ErrServerStopped) {

			}
			panic(err)
		}
	}()
	g.commandStream, err = g.connector.ReadCommands(context.Background(), &emptypb.Empty{})
	if err != nil {
		return nil, err
	}
	go func() {
		err := readCommandStream(g.commandStream, func(message *messages.CommandPacket) error {
			if message.Timestamp == nil {
				return nil
			}
			log.Println("Getting command")
			g.incomingChannel <- message
			return nil
		})
		if !errors.Is(err, io.EOF) {
			panic(err)
		}
	}()
	return confirmation.BotUser, nil
}

func (g *grpcConnectorRelay) Send(message *messages.BotPacket) error {
	ctx, _ := context.WithTimeout(context.Background(), 5*time.Second)
	_, err := g.connector.SendMessage(ctx, message)
	return err
}

func (g *grpcConnectorRelay) Recv() (*relay.ConnectorMessage, error) {
	var p relay.ConnectorMessage
	err := g.RecvMsg(&p)
	return &p, err
}

func (g *grpcConnectorRelay) RecvMsg(packet *relay.ConnectorMessage) error {
	var ok bool
	packet.Message, ok = <-g.incomingChannel
	if !ok {
		return io.EOF
	}
	return nil
}
