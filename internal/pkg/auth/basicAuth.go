package auth

import (
	"context"
	"errors"
	"github.com/raf924/bot-grpc-relay/pkg/utils"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/status"
)

type basicAuth struct {
	users []string
}

func (a *basicAuth) Intercept(ctx context.Context) error {
	err := a.authorize(ctx)
	if err != nil {
		err = status.Error(codes.PermissionDenied, err.Error())
	}
	return err
}

func (a *basicAuth) authorize(ctx context.Context) error {
	if len(a.users) == 0 {
		return nil
	}
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

func BasicAuth(users []string) *basicAuth {
	return &basicAuth{users: users}
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
