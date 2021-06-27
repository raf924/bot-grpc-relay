package auth

import (
	"context"
	"errors"
	"github.com/google/uuid"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"sync"
)

type session struct {
	sessions map[string]struct{}
	mutex    *sync.Mutex
}

func (i *session) Intercept(ctx context.Context) error {
	err := i.identify(ctx)
	if err != nil {
		err = status.Error(codes.PermissionDenied, err.Error())
	}
	return err
}

func (i *session) identify(ctx context.Context) error {
	method, ok := grpc.Method(ctx)
	if !ok {
		return nil
	}
	if method == "/connector.Connector/Connect" {
		i.mutex.Lock()
		sessionId := uuid.New().String()
		i.sessions[sessionId] = struct{}{}
		i.mutex.Unlock()
		return grpc.SetHeader(ctx, metadata.Pairs("sessionid", sessionId))
	}
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return errors.New("no metadata")
	}
	values := md.Get("sessionId")
	if len(values) == 0 {
		return errors.New("no sessionId")
	}
	for _, value := range values {
		if _, ok := i.sessions[value]; ok {
			return nil
		}
	}
	return errors.New("found no valid session")
}

func Session() *session {
	return &session{map[string]struct{}{}, &sync.Mutex{}}
}
