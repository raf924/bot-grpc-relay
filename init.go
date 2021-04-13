package pkg

import (
	"github.com/raf924/bot-grpc-relay/internal/pkg/bot"
	_ "github.com/raf924/bot-grpc-relay/internal/pkg/bot"
	"github.com/raf924/bot-grpc-relay/internal/pkg/config"
	"github.com/raf924/bot-grpc-relay/internal/pkg/connector"
	"github.com/raf924/bot/pkg/queue"
	"github.com/raf924/bot/pkg/relay/client"
	"github.com/raf924/bot/pkg/relay/server"
)

func init() {
	server.RegisterRelayServer("grpc", func(config interface{}, connectorExchange *queue.Exchange) server.RelayServer {
		return connector.NewGrpcRelayServer(config, connectorExchange)
	})
	client.RegisterRelayClient("grpc", func(config interface{}, withBotExchange *queue.Exchange) client.RelayClient {
		return bot.NewGrpcRelayClient(config, withBotExchange)
	})
}

var NewGrpcRelayServer = connector.NewGrpcRelayServer

type GrpcServerConfig = config.GrpcServerConfig
type GrpcClientConfig = config.GrpcClientConfig
