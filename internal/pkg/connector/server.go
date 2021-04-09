package connector

import (
	"context"
	"github.com/golang/protobuf/ptypes/empty"
	config2 "github.com/raf924/bot-grpc-relay/internal/pkg/config"
	"github.com/raf924/bot-grpc-relay/pkg/server"
	api "github.com/raf924/connector-api/pkg/gen"
	messages "github.com/raf924/connector-api/pkg/gen"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/reflect/protoreflect"
	"gopkg.in/yaml.v2"
	"io"
	"log"
)

func NewGrpcBotRelay(config interface{}) *grpcBotRelay {
	data, err := yaml.Marshal(config)
	if err != nil {
		panic(err)
	}
	var conf config2.GrpcRelayConfig
	if err := yaml.Unmarshal(data, &conf); err != nil {
		panic(err)
	}
	return &grpcBotRelay{config: conf}
}

type grpcBotRelay struct {
	api.UnimplementedConnectorServer
	config         config2.GrpcRelayConfig
	messageQueue   chan protoreflect.ProtoMessage
	commandQueue   chan protoreflect.ProtoMessage
	eventsQueue    chan protoreflect.ProtoMessage
	registration   *messages.RegistrationPacket
	users          []*messages.User
	botUser        *messages.User
	botChannel     chan *messages.BotPacket
	readyChannel   chan struct{}
	streamsContext context.Context
	streamsCancel  context.CancelFunc
	streams        []grpc.ServerStream
}

func (c *grpcBotRelay) send(ctx context.Context, channel chan protoreflect.ProtoMessage, stream grpc.ServerStream) error {
	defer func() {
		log.Println("Cancelling contexts")
		c.streamsCancel()
	}()
	var errCh = make(chan error, 1)
	go func() {
		var f = func() error {
			for {
				select {
				case packet, ok := <-channel:
					if !ok {
						return io.EOF
					}
					log.Println("sending packet")
					if err := stream.SendMsg(packet); err != nil {
						return err
					}
					log.Println("packet sent")
				case <-stream.Context().Done():
					return stream.Context().Err()
				case <-ctx.Done():
					return ctx.Err()
				}
			}
		}
		errCh <- f()
	}()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case err := <-errCh:
			return err
		}
	}
}

func (c *grpcBotRelay) RecvMsg(packet *messages.BotPacket) error {
	if c.botChannel == nil {
		return io.ErrClosedPipe
	}
	p, ok := <-c.botChannel
	if !ok {
		return io.EOF
	}
	*packet = *p
	return nil
}

func (c *grpcBotRelay) Commands() []*messages.Command {
	if c.registration == nil {
		return nil
	}
	return c.registration.Commands
}

func (c *grpcBotRelay) Start(botUser *messages.User, users []*messages.User) error {
	c.botUser = botUser
	c.users = users
	c.readyChannel = make(chan struct{})
	c.botChannel = make(chan *messages.BotPacket)
	c.messageQueue = make(chan protoreflect.ProtoMessage)
	c.commandQueue = make(chan protoreflect.ProtoMessage)
	c.eventsQueue = make(chan protoreflect.ProtoMessage)
	return server.StartConnectorServer(c, c.config)
}

func (c *grpcBotRelay) Trigger() string {
	if c.registration == nil {
		return ""
	}
	return c.registration.Trigger
}

func (c *grpcBotRelay) Ready() <-chan struct{} {
	return c.readyChannel
}

func (c *grpcBotRelay) Register(ctx context.Context, registration *messages.RegistrationPacket) (*messages.ConfirmationPacket, error) {
	log.Println("Registering new client")
	c.registration = registration
	go func() {
		c.readyChannel <- struct{}{}
	}()
	if c.streamsCancel != nil {
		log.Println("Cancelling contexts")
		c.streamsCancel()
	}
	c.streamsContext, c.streamsCancel = context.WithCancel(context.Background())
	return &messages.ConfirmationPacket{
		BotUser: c.botUser,
		Users:   c.users,
	}, nil
}

func (c *grpcBotRelay) Ping(context.Context, *empty.Empty) (*empty.Empty, error) {
	return &empty.Empty{}, nil
}

func (c *grpcBotRelay) ReadMessages(empty *empty.Empty, server api.Connector_ReadMessagesServer) error {
	ctx, _ := context.WithCancel(c.streamsContext)
	return c.send(ctx, c.messageQueue, server)
}

func (c *grpcBotRelay) ReadCommands(empty *empty.Empty, server api.Connector_ReadCommandsServer) error {
	ctx, _ := context.WithCancel(c.streamsContext)
	return c.send(ctx, c.commandQueue, server)
}

func (c *grpcBotRelay) ReadUserEvents(empty *empty.Empty, server api.Connector_ReadUserEventsServer) error {
	ctx, _ := context.WithCancel(c.streamsContext)
	return c.send(ctx, c.eventsQueue, server)
}

func (c *grpcBotRelay) SendMessage(ctx context.Context, packet *messages.BotPacket) (*empty.Empty, error) {
	log.Println("Message received")
	c.botChannel <- packet
	return &empty.Empty{}, nil
}

func (c *grpcBotRelay) PassMessage(message *messages.MessagePacket) error {
	c.messageQueue <- message
	return nil
}

func (c *grpcBotRelay) PassEvent(event *messages.UserPacket) error {
	c.eventsQueue <- event
	switch event.Event {
	case messages.UserEvent_JOINED:
		c.users = append(c.users, event.GetUser())
	case messages.UserEvent_LEFT:
		for i, user := range c.users {
			if user.GetNick() == event.GetUser().GetNick() {
				c.users = append(c.users[:i], c.users[i+1:]...)
				break
			}
		}
	}
	return nil
}

func (c *grpcBotRelay) PassCommand(command *messages.CommandPacket) error {
	log.Println("passing command")
	c.commandQueue <- command
	return nil
}
