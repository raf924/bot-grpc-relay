package connector

import (
	"context"
	"github.com/golang/protobuf/ptypes/empty"
	api "github.com/raf924/connector-api/pkg/gen"
	messages "github.com/raf924/connector-api/pkg/gen"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/reflect/protoreflect"
	"io"
	"log"
)

type connectorServer struct {
	api.UnimplementedConnectorServer
	registration   *messages.RegistrationPacket
	readyChannel   chan struct{}
	streamsContext context.Context
	streamsCancel  context.CancelFunc
	botUser        *messages.User
	users          []*messages.User
	messageQueue   chan protoreflect.ProtoMessage
	commandQueue   chan protoreflect.ProtoMessage
	eventsQueue    chan protoreflect.ProtoMessage
	botChannel     chan *messages.BotPacket
}

func NewConnectorServer(readyChannel chan struct{}, botUser *messages.User, users []*messages.User, messageQueue chan protoreflect.ProtoMessage, commandQueue chan protoreflect.ProtoMessage, eventsQueue chan protoreflect.ProtoMessage, botChannel chan *messages.BotPacket) *connectorServer {
	return &connectorServer{readyChannel: readyChannel, botUser: botUser, users: users, messageQueue: messageQueue, commandQueue: commandQueue, eventsQueue: eventsQueue, botChannel: botChannel}
}

func (c *connectorServer) Register(ctx context.Context, registration *messages.RegistrationPacket) (*messages.ConfirmationPacket, error) {
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

func (c *connectorServer) ReadMessages(empty *empty.Empty, server messages.Connector_ReadMessagesServer) error {
	ctx, _ := context.WithCancel(c.streamsContext)
	return c.send(ctx, c.messageQueue, server)
}

func (c *connectorServer) ReadCommands(empty *empty.Empty, server messages.Connector_ReadCommandsServer) error {
	ctx, _ := context.WithCancel(c.streamsContext)
	return c.send(ctx, c.commandQueue, server)
}

func (c *connectorServer) ReadUserEvents(empty *empty.Empty, server messages.Connector_ReadUserEventsServer) error {
	ctx, _ := context.WithCancel(c.streamsContext)
	return c.send(ctx, c.eventsQueue, server)
}

func (c *connectorServer) SendMessage(ctx context.Context, packet *messages.BotPacket) (*empty.Empty, error) {
	log.Println("Message received")
	c.botChannel <- packet
	return &empty.Empty{}, nil
}

func (c *connectorServer) send(ctx context.Context, channel chan protoreflect.ProtoMessage, stream grpc.ServerStream) error {
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
