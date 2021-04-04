package pkg

import (
	_ "github.com/raf924/bot-grpc-relay/internal/pkg/bot"
	"github.com/raf924/bot-grpc-relay/internal/pkg/config"
	"github.com/raf924/bot-grpc-relay/internal/pkg/connector"
	"github.com/raf924/bot/pkg/relay"
)

func init() {
	relay.RegisterBotRelay("grpc", func(config interface{}) relay.BotRelay {
		return NewGrpcBotRelay(config)
	})
}

var NewGrpcBotRelay = connector.NewGrpcBotRelay

type GrpcRelayConfig = config.GrpcRelayConfig
