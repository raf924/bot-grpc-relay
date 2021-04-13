package connector

import (
	"context"
	"fmt"
	"github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/ptypes/empty"
	"github.com/raf924/bot-grpc-relay/internal/pkg/config"
	"github.com/raf924/bot/pkg/queue"
	"github.com/raf924/connector-api/pkg/gen"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/test/bufconn"
	"google.golang.org/protobuf/types/known/timestamppb"
	"net"
	"testing"
	"time"
)

func setupGrpcRelayServer(t testing.TB, connectorExchange *queue.Exchange) gen.ConnectorClient {
	grpcRelay := newGrpcRelayServer(config.GrpcServerConfig{
		Port:    0,
		Timeout: "30s",
		TLS: struct {
			Enabled bool     `yaml:"enabled"`
			Ca      string   `yaml:"ca"`
			Cert    string   `yaml:"cert"`
			Key     string   `yaml:"key"`
			Users   []string `yaml:"users"`
		}{
			Enabled: false,
		},
	}, connectorExchange)
	l := bufconn.Listen(1024 * 1024)
	err := grpcRelay.start(&gen.User{
		Nick:  "bot",
		Id:    "id",
		Mod:   false,
		Admin: false,
	}, []*gen.User{}, "", l)
	if err != nil {
		t.Fatalf("Unexpected error = %v", err)
	}
	dialOption := grpc.WithContextDialer(func(ctx context.Context, s string) (net.Conn, error) {
		return l.Dial()
	})
	conn, err := grpc.Dial(fmt.Sprintf("%s:%d", "127.0.0.1", 0), dialOption, grpc.WithInsecure(), grpc.WithBlock())
	if err != nil {
		t.Fatalf("Unexpected error = %v", err)
	}
	return gen.NewConnectorClient(conn)
}

func TestGrpcRelayServer_SendMessage(t *testing.T) {
	var b2c = queue.NewQueue()
	var c2b = queue.NewQueue()
	connectorExchange, _ := queue.NewExchange(b2c, c2b)
	botExchange, _ := queue.NewExchange(c2b, b2c)
	client := setupGrpcRelayServer(t, connectorExchange)
	var header metadata.MD
	_, err := client.Register(context.Background(), &gen.RegistrationPacket{
		Trigger:  "",
		Commands: nil,
	}, grpc.Header(&header))
	if err != nil {
		t.Fatalf("Unexpected error = %v", err)
	}
	ctx := metadata.NewOutgoingContext(context.Background(), header)
	_, err = client.SendMessage(ctx, &gen.BotPacket{
		Timestamp: timestamppb.Now(),
		Message:   "test",
		Recipient: nil,
		Private:   false,
	})
	if err != nil {
		t.Fatalf("Unexpected error = %v", err)
	}
	m, err := botExchange.Consume()
	if err != nil {
		t.Fatalf("Unexpected error = %v", err)
	}
	switch m.(type) {
	case *gen.BotPacket:
		if m.(*gen.BotPacket).GetMessage() != "test" {
			t.Errorf("Expected %v got %v", "test", m.(*gen.BotPacket).GetMessage())
		}
	default:
		t.Errorf("Expected BotPacket")
	}
}

type receiver func(client grpc.ClientStream) (proto.Message, error)
type reader func(client gen.ConnectorClient, ctx context.Context) (grpc.ClientStream, error)

type test struct {
	packet   proto.Message
	reader   reader
	receiver receiver
}

var botUser = &gen.User{
	Nick:  "bot",
	Id:    "id",
	Mod:   false,
	Admin: false,
}

func testReadPackets(t *testing.T, test test) {
	var b2c = queue.NewQueue()
	var c2b = queue.NewQueue()
	connectorExchange, _ := queue.NewExchange(b2c, c2b)
	botExchange, _ := queue.NewExchange(c2b, b2c)
	client := setupGrpcRelayServer(t, connectorExchange)
	var header metadata.MD
	_, err := client.Register(context.Background(), &gen.RegistrationPacket{
		Trigger:  "",
		Commands: nil,
	}, grpc.Header(&header))
	if err != nil {
		t.Fatalf("Unexpected error = %v", err)
	}
	ctx := metadata.NewOutgoingContext(context.Background(), header)
	mClient, err := test.reader(client, ctx)
	if err != nil {
		t.Fatalf("unexpected error = %v", err)
	}
	go func() {
		<-mClient.Context().Done()
		if mClient.Context().Err() != nil {
			t.Errorf("unexpected error = %v", err)
			return
		}
	}()
	time.Sleep(500 * time.Microsecond)
	err = botExchange.Produce(test.packet)
	if err != nil {
		t.Fatalf("unexpected error = %v", err)
	}
	m, err := test.receiver(mClient)
	if err != nil {
		t.Fatalf("unexpected error = %v", err)
	}
	if test.packet.String() != m.String() {
		t.Fatalf("expected `%v` got `%v`", test.packet, m)
	}
}

var messageTest = test{
	packet: &gen.MessagePacket{
		Timestamp: timestamppb.Now(),
		Message:   "test",
		User:      botUser,
		Private:   false,
	},
	reader: func(client gen.ConnectorClient, ctx context.Context) (grpc.ClientStream, error) {
		return client.ReadMessages(ctx, &empty.Empty{})
	},
	receiver: func(client grpc.ClientStream) (proto.Message, error) {
		var p gen.MessagePacket
		return &p, client.RecvMsg(&p)
	},
}

var commandTest = test{
	packet: &gen.CommandPacket{
		Timestamp: timestamppb.Now(),
		Command:   "echo",
		Args:      nil,
		User:      botUser,
		Private:   false,
		ArgString: "",
	},
	reader: func(client gen.ConnectorClient, ctx context.Context) (grpc.ClientStream, error) {
		return client.ReadCommands(ctx, &empty.Empty{})
	},
	receiver: func(client grpc.ClientStream) (proto.Message, error) {
		var m gen.CommandPacket
		return &m, client.RecvMsg(&m)
	},
}

var eventTest = test{
	packet: &gen.UserPacket{
		Timestamp: timestamppb.Now(),
		User:      botUser,
		Event:     gen.UserEvent_JOINED,
	},
	reader: func(client gen.ConnectorClient, ctx context.Context) (grpc.ClientStream, error) {
		return client.ReadUserEvents(ctx, &empty.Empty{})
	},
	receiver: func(client grpc.ClientStream) (proto.Message, error) {
		var e gen.UserPacket
		return &e, client.RecvMsg(&e)
	},
}

func TestGrpcBotRelay_Read(t *testing.T) {
	t.Run("Messages", func(t *testing.T) {
		testReadPackets(t, messageTest)
	})
	t.Run("Commands", func(t *testing.T) {
		testReadPackets(t, commandTest)
	})
	t.Run("Events", func(t *testing.T) {
		testReadPackets(t, eventTest)
	})
}
