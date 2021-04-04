package interceptors

import (
	"context"
	"google.golang.org/grpc"
)

type ContextInterceptorFunc func(ctx context.Context) error
type ContextInterceptor interface {
	Intercept(ctx context.Context) error
}

func unaryInterceptor(ci ContextInterceptor) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (resp interface{}, err error) {
		if err := ci.Intercept(ctx); err != nil {
			return nil, err
		}
		return handler(ctx, req)
	}
}

func streamInterceptor(ci ContextInterceptor) grpc.StreamServerInterceptor {
	return func(srv interface{}, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		if err := ci.Intercept(ss.Context()); err != nil {
			return err
		}
		return handler(srv, ss)
	}
}

func OptionsFrom(ci ContextInterceptor) []grpc.ServerOption {
	return []grpc.ServerOption{
		grpc.ChainUnaryInterceptor(unaryInterceptor(ci)),
		grpc.ChainStreamInterceptor(streamInterceptor(ci)),
	}
}
