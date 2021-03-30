package connector

import (
	"context"
	"errors"
	"github.com/raf924/bot-grpc-relay/internal/pkg/utils"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/peer"
)

type basicAuth struct {
	users []string
}

func (a *basicAuth) Authorize(ctx context.Context) error {
	client, ok := peer.FromContext(ctx)
	if !ok {
		return errors.New("couldn't access client information")
	}
	if client.AuthInfo.AuthType() != "tls" {
		return errors.New("connection is not secure")
	}
	tlsInfo := client.AuthInfo.(credentials.TLSInfo)
	if len(tlsInfo.State.VerifiedChains) > 0 {
		return nil
	}
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return errors.New("couldn't access metadata")
	}
	user := md.Get("authorization")
	if len(user) == 0 {
		return errors.New("no authorization information")
	}
	for _, u := range a.users {
		if utils.CheckHash(user[0], u) {
			return nil
		}
	}
	return errors.New("invalid user")
}

func (a *basicAuth) UnaryInterceptor() grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (resp interface{}, err error) {
		if err := a.Authorize(ctx); err != nil {
			return nil, err
		}
		return handler(ctx, req)
	}
}

func (a *basicAuth) StreamInterceptor() grpc.StreamServerInterceptor {
	return func(srv interface{}, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		if err := a.Authorize(ss.Context()); err != nil {
			return err
		}
		return handler(srv, ss)
	}
}

func BasicAuth(users []string) []grpc.ServerOption {
	if len(users) == 0 {
		return nil
	}
	a := &basicAuth{users: users}
	return []grpc.ServerOption{
		grpc.UnaryInterceptor(a.UnaryInterceptor()),
		grpc.StreamInterceptor(a.StreamInterceptor()),
	}
}

type basicAuthCredentials struct {
	user string
}

func (b *basicAuthCredentials) GetRequestMetadata(ctx context.Context, uri ...string) (map[string]string, error) {
	return map[string]string{"authorization": b.user}, nil
}

func (b *basicAuthCredentials) RequireTransportSecurity() bool {
	return true
}

func NewBasicAuthCreds(user string) credentials.PerRPCCredentials {
	return &basicAuthCredentials{
		user: user,
	}
}
