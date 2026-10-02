package server

import (
	"context"
	"time"

	"google.golang.org/grpc"
)

func WithGRPCListenHost(host string) Option {
	return func(o *serverOpts) {
		o.GRPCListenHost = host
	}
}

func WithGRPCDisabled() Option {
	return func(o *serverOpts) {
		o.GRPCDisabled = true
	}
}

func WithReflection(enabled bool) Option {
	return func(o *serverOpts) {
		o.EnableReflection = enabled
	}
}

func WithGRPCTracing(i grpc.UnaryServerInterceptor) Option {
	return func(o *serverOpts) {
		o.GRPCTracing = i
	}
}

func WithDrain(fn func(ctx context.Context), delay time.Duration) Option {
	return func(o *serverOpts) {
		o.DrainFn = fn
		o.DrainDelay = delay
	}
}
