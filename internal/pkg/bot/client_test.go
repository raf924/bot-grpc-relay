package bot

import (
	"context"
	"fmt"
	"github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/ptypes/empty"
	"github.com/raf924/bot-grpc-relay/internal/pkg/config"
	"github.com/raf924/bot-grpc-relay/pkg/server"
	"github.com/raf924/bot/pkg/queue"
	messages "github.com/raf924/connector-api/pkg/gen"
	"google.golang.org/grpc"
	"google.golang.org/grpc/test/bufconn"
	"google.golang.org/protobuf/types/known/timestamppb"
	"net"
	"testing"
)

var botUser = &messages.User{
	Nick:  "bot",
	Id:    "botId",
	Mod:   false,
	Admin: false,
}

var user = &messages.User{
	Nick:  "user",
	Id:    "userId",
	Mod:   false,
	Admin: false,
}

var commandReply = &messages.CommandPacket{
	Timestamp: timestamppb.Now(),
	Command:   "test",
	Args:      nil,
	User:      user,
	Private:   false,
	ArgString: "",
}

var messageReply = &messages.MessagePacket{
	Timestamp: timestamppb.Now(),
	Message:   "test",
	User:      user,
	Private:   false,
}

var userEventReply = &messages.UserPacket{
	Timestamp: timestamppb.Now(),
	User: &messages.User{
		Nick:  "newUser",
		Id:    "newId",
		Mod:   false,
		Admin: false,
	},
	Event: 0,
}

type dummyServer struct {
	messages.UnimplementedConnectorServer
}

func (d *dummyServer) Register(ctx context.Context, packet *messages.RegistrationPacket) (*messages.ConfirmationPacket, error) {
	return &messages.ConfirmationPacket{
		BotUser: botUser,
		Users:   []*messages.User{botUser, user},
	}, nil
}

func (d *dummyServer) ReadMessages(empty *empty.Empty, server messages.Connector_ReadMessagesServer) error {
	err := server.Send(messageReply)
	return err
}

func (d *dummyServer) ReadCommands(empty *empty.Empty, server messages.Connector_ReadCommandsServer) error {
	err := server.Send(commandReply)
	return err
}

func (d *dummyServer) ReadUserEvents(empty *empty.Empty, server messages.Connector_ReadUserEventsServer) error {
	err := server.Send(userEventReply)
	return err
}

func (d *dummyServer) SendMessage(ctx context.Context, packet *messages.BotPacket) (*empty.Empty, error) {
	return &empty.Empty{}, nil
}

func (d *dummyServer) Ping(ctx context.Context, empty *empty.Empty) (*empty.Empty, error) {
	return empty, nil
}

func setupGrpcClient(t testing.TB, botExchange *queue.Exchange) *grpcRelayClient {
	grpcRelay := newGrpcRelayClient(config.GrpcClientConfig{}, botExchange)
	l := bufconn.Listen(1024 * 1024)
	err := server.StartServer(l, &dummyServer{}, config.GrpcServerConfig{})
	if err != nil {
		t.Fatalf("unexpected error = %v", err)
	}
	dialOption := grpc.WithContextDialer(func(ctx context.Context, s string) (net.Conn, error) {
		return l.Dial()
	})
	conn, err := grpc.Dial(fmt.Sprintf("%s:%d", "127.0.0.1", 0), dialOption, grpc.WithInsecure(), grpc.WithBlock())
	if err != nil {
		t.Fatalf("Unexpected error = %v", err)
	}
	_, err = grpcRelay.connect(&messages.RegistrationPacket{
		Trigger:  "",
		Commands: nil,
	}, conn)
	if err != nil {
		t.Fatalf("Unexpected error = %v", err)
	}
	return grpcRelay
}

func TestGrpcRelayClient(t *testing.T) {
	var b2c = queue.NewQueue()
	var c2b = queue.NewQueue()
	clientExchange, _ := queue.NewExchange(b2c, c2b)
	botExchange, _ := queue.NewExchange(c2b, b2c)
	_ = setupGrpcClient(t, botExchange)
	packet1, err := clientExchange.Consume()
	if err != nil {
		t.Fatalf("Unexpected error = %v", err)
	}
	packet2, err := clientExchange.Consume()
	if err != nil {
		t.Fatalf("Unexpected error = %v", err)
	}
	packet3, err := clientExchange.Consume()
	if err != nil {
		t.Fatalf("Unexpected error = %v", err)
	}
	var replies = []string{messageReply.String(), commandReply.String(), userEventReply.String()}
	var packets = map[string]struct{}{packet1.(proto.Message).String(): {}, packet2.(proto.Message).String(): {}, packet3.(proto.Message).String(): {}}
	for _, reply := range replies {
		if _, ok := packets[reply]; !ok {
			t.Errorf("reply not received: %v", reply)
		}
		delete(packets, reply)
	}
	if len(packets) > 0 {
		t.Fatalf("all replies should have been consumed: remaining %v", packets)
	}
}
