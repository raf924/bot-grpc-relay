package pkg

import (
	_ "github.com/raf924/bot-grpc-relay/internal/pkg/bot"
	"github.com/raf924/bot-grpc-relay/internal/pkg/connector"
	"github.com/raf924/bot/pkg/relay"
)

func init() {
	relay.RegisterBotRelay("grpc", NewGrpcBotRelay)
}

var NewGrpcBotRelay = connector.NewGrpcBotRelay

type GrpcRelayConfig = connector.GrpcRelayConfig
