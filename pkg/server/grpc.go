package server

import (
	"github.com/raf924/bot-grpc-relay/internal/pkg/auth"
	"github.com/raf924/bot-grpc-relay/internal/pkg/config"
	"github.com/raf924/bot-grpc-relay/internal/pkg/interceptors"
	"github.com/raf924/bot-grpc-relay/pkg/utils"
	api "github.com/raf924/connector-api/pkg/gen"
	messages "github.com/raf924/connector-api/pkg/gen"
	"google.golang.org/grpc"
	"log"
	"net"
	"strconv"
)

func StartConnectorServer(c messages.ConnectorServer, config config.GrpcServerConfig) error {
	log.Println("Listening on ", config.Port)
	l, err := net.Listen("tcp", net.JoinHostPort("", strconv.FormatInt(int64(config.Port), 10)))
	if err != nil {
		return err
	}
	return StartServer(l, c, config)
}

func StartServer(l net.Listener, c messages.ConnectorServer, config config.GrpcServerConfig) error {
	var err error
	var tlsOption grpc.ServerOption = grpc.EmptyServerOption{}
	if config.TLS.Enabled {
		log.Println("Using TLS Server Configuration")
		tlsOption, err = utils.LoadTLSServerConfig(config.TLS.Ca, config.TLS.Cert, config.TLS.Key)
		if err != nil {
			return err
		}
	}

	var authorizedUsers []string
	if config.TLS.Enabled {
		authorizedUsers = config.TLS.Users
	}
	basicAuthOptions := interceptors.OptionsFrom(auth.BasicAuth(authorizedUsers))
	sessionOptions := interceptors.OptionsFrom(auth.Session())
	grpcServer := grpc.NewServer(append(basicAuthOptions, append(sessionOptions, tlsOption)...)...)
	api.RegisterConnectorServer(grpcServer, c)
	go func() {
		if err := grpcServer.Serve(l); err != nil {
			panic(err)
		}
		return
	}()
	return nil
}
