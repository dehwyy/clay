package server

import (
	"context"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	grpc_middleware "github.com/grpc-ecosystem/go-grpc-middleware"
	"github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
	"github.com/not-for-prod/clay/server/middlewares/mwhttp"
	"google.golang.org/grpc"
)

// Option is an optional setting applied to the Server.
type Option func(*serverOpts)

type serverOpts struct {
	RPCPort int
	// If HTTPPort is the same then muxing listener is created.
	HTTPPort int
	HTTPMux  *chi.Mux

	HTTPMiddlewares []func(http.Handler) http.Handler

	HTTPServerFns []func(*http.Server)
	HTTPRoutes    []func(chi.Router)

	GRPCOpts             []grpc.ServerOption
	GRPCUnary            []grpc.UnaryServerInterceptor
	GRPCTracing          grpc.UnaryServerInterceptor
	GRPCUnaryInterceptor grpc.UnaryServerInterceptor
	GRPCListenHost       string
	GRPCDisabled         bool

	EnableReflection    bool
	RuntimeServeMuxOpts []runtime.ServeMuxOption

	DrainFn    func(ctx context.Context)
	DrainDelay time.Duration
}

func defaultServerOpts(mainPort int) *serverOpts {
	return &serverOpts{
		RPCPort:          mainPort,
		HTTPPort:         mainPort,
		HTTPMux:          chi.NewMux(),
		EnableReflection: true,
	}
}

func (o *serverOpts) finalize() {
	chain := make([]grpc.UnaryServerInterceptor, 0, len(o.GRPCUnary)+1)
	if o.GRPCTracing != nil {
		chain = append(chain, o.GRPCTracing)
	}
	chain = append(chain, o.GRPCUnary...)
	if len(chain) == 0 {
		return
	}
	o.GRPCUnaryInterceptor = grpc_middleware.ChainUnaryServer(chain...)
}

// WithGRPCOpts sets gRPC server options.
func WithGRPCOpts(opts ...grpc.ServerOption) Option {
	return func(o *serverOpts) {
		o.GRPCOpts = append(o.GRPCOpts, opts...)
	}
}

// WithHTTPPort sets HTTP RPC port to listen on.
// Set same port as main to use single port.
func WithHTTPPort(port int) Option {
	return func(o *serverOpts) {
		o.HTTPPort = port
	}
}

// WithHTTPMiddlewares sets up HTTP middlewares to work with.
func WithHTTPMiddlewares(mws ...mwhttp.Middleware) Option {
	mwGeneric := make([]func(http.Handler) http.Handler, 0, len(mws))
	for _, mw := range mws {
		mwGeneric = append(mwGeneric, mw)
	}
	return func(o *serverOpts) {
		o.HTTPMiddlewares = mwGeneric
	}
}

// WithGRPCUnaryMiddlewares sets up unary middlewares for gRPC server.
func WithGRPCUnaryMiddlewares(mws ...grpc.UnaryServerInterceptor) Option {
	return func(o *serverOpts) {
		o.GRPCUnary = append(o.GRPCUnary, mws...)
	}
}

// WithGRPCMiddlewares sets up unary middlewares for gRPC server.
func WithGRPCMiddlewares(mws ...grpc.UnaryServerInterceptor) Option {
	return WithGRPCUnaryMiddlewares(mws...)
}

// WithGRPCStreamMiddlewares sets up stream middlewares for gRPC server.
func WithGRPCStreamMiddlewares(mws ...grpc.StreamServerInterceptor) Option {
	return func(o *serverOpts) {
		o.GRPCOpts = append(o.GRPCOpts, grpc.StreamInterceptor(grpc_middleware.ChainStreamServer(mws...)))
	}
}

// WithHTTPMux sets existing HTTP muxer to use instead of creating new one.
func WithHTTPMux(mux *chi.Mux) Option {
	return func(o *serverOpts) {
		o.HTTPMux = mux
	}
}

func WithRuntimeServeMuxOpts(opts ...runtime.ServeMuxOption) Option {
	return func(o *serverOpts) {
		o.RuntimeServeMuxOpts = append(o.RuntimeServeMuxOpts, opts...)
	}
}
