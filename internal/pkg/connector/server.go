package connector

import (
	"context"
	"github.com/golang/protobuf/ptypes/empty"
	cnf "github.com/raf924/bot-grpc-relay/internal/pkg/config"
	"github.com/raf924/bot-grpc-relay/pkg/server"
	"github.com/raf924/bot/pkg/queue"
	"github.com/raf924/bot/pkg/users"
	api "github.com/raf924/connector-api/pkg/gen"
	messages "github.com/raf924/connector-api/pkg/gen"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"gopkg.in/yaml.v2"
	"log"
	"net"
)

func NewGrpcRelayServer(config interface{}, connectorExchange *queue.Exchange) *grpcRelayServer {
	data, err := yaml.Marshal(config)
	if err != nil {
		panic(err)
	}
	var conf cnf.GrpcServerConfig
	if err := yaml.Unmarshal(data, &conf); err != nil {
		panic(err)
	}
	return newGrpcRelayServer(conf, connectorExchange)
}

func newGrpcRelayServer(config cnf.GrpcServerConfig, connectorExchange *queue.Exchange) *grpcRelayServer {
	return &grpcRelayServer{config: config, consumerSessions: map[string]*queue.Consumer{}}
}

type grpcRelayServer struct {
	api.UnimplementedConnectorServer
	config           cnf.GrpcServerConfig
	commands         []*messages.Command
	consumerSessions map[string]*queue.Consumer
	clientConsumer   *queue.Consumer
	clientProducer   *queue.Producer
	messageQueue     queue.Queue
	commandQueue     queue.Queue
	eventsQueue      queue.Queue
	messageProducer  *queue.Producer
	commandProducer  *queue.Producer
	eventsProducer   *queue.Producer
	registration     *messages.RegistrationPacket
	users            *users.UserList
	botUser          *messages.User
	streamsContext   context.Context
	streamsCancel    context.CancelFunc
	trigger          string
}

func (c *grpcRelayServer) Send(message proto.Message) error {
	switch message.(type) {
	case *messages.MessagePacket:
		return c.dispatchMessage(message.(*messages.MessagePacket))
	case *messages.CommandPacket:
		return c.dispatchCommand(message.(*messages.CommandPacket))
	case *messages.UserPacket:
		return c.dispatchEvent(message.(*messages.UserPacket))
	default:
		log.Println("Unknown type")
	}
	return nil
}

func (c *grpcRelayServer) Recv() (*messages.BotPacket, error) {
	p, err := c.clientConsumer.Consume()
	return p.(*messages.BotPacket), err
}

func (c *grpcRelayServer) relay(consumer *queue.Consumer, stream grpc.ServerStream) error {
	defer func() {
		consumer.Cancel()
	}()
	var errCh = make(chan error, 1)
	go func() {
		var f = func() error {
			for {
				select {
				case <-stream.Context().Done():
					return stream.Context().Err()
				default:
					packet, err := consumer.Consume()
					if err != nil {
						return err
					}
					if err = stream.SendMsg(packet); err != nil {
						return err
					}
				}
			}
		}
		errCh <- f()
	}()
	err := <-errCh
	return err
}

func (c *grpcRelayServer) Commands() []*messages.Command {
	return c.commands
}

func (c *grpcRelayServer) Start(botUser *messages.User, onlineUsers []*messages.User, trigger string) error {
	return c.start(botUser, onlineUsers, trigger, nil)
}

func (c *grpcRelayServer) start(botUser *messages.User, onlineUsers []*messages.User, trigger string, l net.Listener) error {
	c.trigger = trigger
	c.botUser = botUser
	c.users = users.NewUserList(onlineUsers...)

	c.messageQueue = queue.NewQueue()
	c.commandQueue = queue.NewQueue()
	c.eventsQueue = queue.NewQueue()
	var err error
	c.messageProducer, err = c.messageQueue.NewProducer()
	if err != nil {
		return err
	}
	c.commandProducer, err = c.commandQueue.NewProducer()
	if err != nil {
		return err
	}
	c.eventsProducer, err = c.eventsQueue.NewProducer()
	if err != nil {
		return err
	}

	clientQueue := queue.NewQueue()
	c.clientProducer, err = clientQueue.NewProducer()
	c.clientConsumer, err = clientQueue.NewConsumer()

	if l == nil {
		return server.StartConnectorServer(c, c.config)
	}
	return server.StartServer(l, c, c.config)
}

func (c *grpcRelayServer) Trigger() string {
	return c.trigger
}

func (c *grpcRelayServer) Register(ctx context.Context, registration *messages.RegistrationPacket) (*messages.ConfirmationPacket, error) {
	log.Println("Registering new client")
	c.commands = append(c.commands, registration.GetCommands()...)
	return &messages.ConfirmationPacket{
		BotUser: c.botUser,
		Users:   c.users.All(),
	}, nil
}

func (c *grpcRelayServer) Ping(context.Context, *empty.Empty) (*empty.Empty, error) {
	return &empty.Empty{}, nil
}

func (c *grpcRelayServer) ReadMessages(empty *empty.Empty, server api.Connector_ReadMessagesServer) error {
	consumer, err := c.messageQueue.NewConsumer()
	if err != nil {
		return status.Error(codes.FailedPrecondition, err.Error())
	}
	return c.relay(consumer, server)
}

func (c *grpcRelayServer) ReadCommands(empty *empty.Empty, server api.Connector_ReadCommandsServer) error {
	consumer, err := c.commandQueue.NewConsumer()
	if err != nil {
		return status.Error(codes.FailedPrecondition, err.Error())
	}
	return c.relay(consumer, server)
}

func (c *grpcRelayServer) ReadUserEvents(empty *empty.Empty, server api.Connector_ReadUserEventsServer) error {
	consumer, err := c.eventsQueue.NewConsumer()
	if err != nil {
		return status.Error(codes.FailedPrecondition, err.Error())
	}
	return c.relay(consumer, server)
}

func (c *grpcRelayServer) SendMessage(ctx context.Context, packet *messages.BotPacket) (*empty.Empty, error) {
	err := c.clientProducer.Produce(packet)
	return &empty.Empty{}, err
}

func (c *grpcRelayServer) dispatchMessage(message *messages.MessagePacket) error {
	return c.messageProducer.Produce(message)
}

func (c *grpcRelayServer) dispatchEvent(event *messages.UserPacket) error {
	err := c.eventsProducer.Produce(event)
	if err != nil {
		return err
	}
	switch event.Event {
	case messages.UserEvent_JOINED:
		c.users.Add(event.GetUser())
	case messages.UserEvent_LEFT:
		c.users.Remove(event.GetUser())
	}
	return nil
}

func (c *grpcRelayServer) dispatchCommand(command *messages.CommandPacket) error {
	return c.commandProducer.Produce(command)
}
