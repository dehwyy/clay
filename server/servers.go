package server

import (
	"bytes"
	"context"
	"io"
	"net/http"

	"github.com/dehwyy/clay/server/clayroute"
	"github.com/dehwyy/clay/transport"
	"github.com/go-chi/chi/v5"
	"github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
	httpSwagger "github.com/swaggo/http-swagger"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"

	"github.com/pkg/errors"
)

type initFunc func() error

func (s *Server) initHTTPServer() error {
	outer := s.opts.HTTPMux

	router := chi.NewMux()
	router.Use(clayroute.Middleware())
	if len(s.opts.HTTPMiddlewares) > 0 {
		router.Use(s.opts.HTTPMiddlewares...)
	}

	router.HandleFunc(
		"/swagger.json", func(w http.ResponseWriter, req *http.Request) {
			io.Copy(w, bytes.NewReader(s.serviceDesc.SwaggerDef()))
		},
	)
	router.HandleFunc(
		"/docs/*", func(w http.ResponseWriter, r *http.Request) {
			httpSwagger.Handler(httpSwagger.URL("swagger.json")).ServeHTTP(w, r)
		},
	)
	router.Get(
		"/docs", func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, "/docs/", http.StatusMovedPermanently)
		},
	)
	router.Get(
		"/docs/swagger.json", func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, "/swagger.json", http.StatusMovedPermanently)
		},
	)

	for _, routes := range s.opts.HTTPRoutes {
		routes(router)
	}

	muxOpts := make([]runtime.ServeMuxOption, 0, len(s.opts.RuntimeServeMuxOpts)+1)
	muxOpts = append(muxOpts, s.opts.RuntimeServeMuxOpts...)
	muxOpts = append(muxOpts, clayroute.GatewayOption())
	mux := runtime.NewServeMux(muxOpts...)

	if err := s.serviceDesc.RegisterHTTP(context.Background(), mux); err != nil {
		return errors.Wrap(err, "couldn't register HTTP server")
	}

	router.Mount("/", mux)

	outer.Mount("/", router)

	httpServer := &http.Server{
		ReadHeaderTimeout: defaultReadHeaderTimeout,
		IdleTimeout:       defaultIdleTimeout,
	}
	for _, fn := range s.opts.HTTPServerFns {
		fn(httpServer)
	}
	httpServer.Handler = outer
	s.httpServer = httpServer

	return nil
}

func (s *Server) initGRPCServer() error {
	grpcOpts := s.opts.GRPCOpts
	if s.opts.GRPCUnaryInterceptor != nil {
		grpcOpts = append(
			append([]grpc.ServerOption(nil), grpcOpts...),
			grpc.UnaryInterceptor(s.opts.GRPCUnaryInterceptor),
		)
	}

	grpcServer := grpc.NewServer(grpcOpts...)
	if s.opts.EnableReflection {
		reflection.Register(grpcServer)
	}

	// apply gRPC interceptor
	if d, ok := s.serviceDesc.(transport.ConfigurableServiceDesc); ok {
		d.Apply(transport.WithUnaryInterceptor(s.opts.GRPCUnaryInterceptor))
	}

	s.serviceDesc.RegisterGRPC(grpcServer)
	s.grpcServer = grpcServer

	return nil
}
