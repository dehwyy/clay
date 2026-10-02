package server

import (
	"context"
	"errors"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/not-for-prod/clay/transport"
	"github.com/soheilhy/cmux"
	"google.golang.org/grpc"
)

// Server is a transport server.
type Server struct {
	opts        *serverOpts
	listeners   *listenerSet
	serviceDesc transport.ServiceDesc
	httpServer  *http.Server
	grpcServer  *grpc.Server

	mu        sync.Mutex
	stopped   bool
	ready     chan struct{}
	readyOnce sync.Once
	stopOnce  sync.Once
	stopErr   error
}

// NewServer creates a Server listening on the rpcPort.
// Pass additional Options to mutate its behaviour.
// By default, HTTP JSON handler and gRPC are listening on the same
// port, admin port is p+2 and profile port is p+4.
func NewServer(rpcPort int, opts ...Option) *Server {
	serverOpts := defaultServerOpts(rpcPort)
	for _, opt := range opts {
		opt(serverOpts)
	}
	serverOpts.finalize()
	return &Server{
		opts:  serverOpts,
		ready: make(chan struct{}),
	}
}

func (s *Server) Ready() <-chan struct{} {
	return s.ready
}

func (s *Server) HTTPAddr() net.Addr {
	if s.listeners == nil || s.listeners.HTTP == nil {
		return nil
	}
	return s.listeners.HTTP.Addr()
}

func (s *Server) GRPCAddr() net.Addr {
	if s.listeners == nil || s.listeners.GRPC == nil {
		return nil
	}
	return s.listeners.GRPC.Addr()
}

// Run starts processing requests to the service.
// It blocks indefinitely, run asynchronously to do anything after that.
func (s *Server) Run(descs ...transport.ServiceDesc) error {
	if err := s.init(descs); err != nil {
		return err
	}
	if s.listeners == nil {
		return nil
	}

	return s.run()
}

func (s *Server) init(descs []transport.ServiceDesc) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.stopped {
		return nil
	}

	s.serviceDesc = transport.NewCompoundServiceDesc(descs...)

	for _, fn := range []initFunc{
		s.initListeners,
		s.initHTTPServer,
		s.initGRPCServer,
	} {
		if err := fn(); err != nil {
			if s.listeners != nil {
				err = errors.Join(
					err,
					s.listeners.closeAll(),
				)
				s.listeners = nil
			}
			return err
		}
	}

	s.readyOnce.Do(func() { close(s.ready) })

	return nil
}

func (s *Server) run() error {
	errChan := make(chan error, 3)
	running := 0

	if s.listeners.mainListener != nil {
		running++
		go func() {
			errChan <- s.listeners.mainListener.Serve()
		}()
	}

	if s.httpServer != nil {
		running++
		go func() {
			errChan <- s.httpServer.Serve(s.listeners.HTTP)
		}()
	}

	if s.grpcServer != nil && s.listeners.GRPC != nil {
		running++
		go func() {
			errChan <- s.grpcServer.Serve(s.listeners.GRPC)
		}()
	}

	for ; running > 0; running-- {
		if err := s.normalizeRunErr(<-errChan); err != nil {
			return err
		}
	}

	return nil
}

func (s *Server) normalizeRunErr(err error) error {
	switch {
	case err == nil,
		errors.Is(err, http.ErrServerClosed),
		errors.Is(err, cmux.ErrServerClosed),
		errors.Is(err, grpc.ErrServerStopped):
		return nil
	case errors.Is(err, net.ErrClosed),
		errors.Is(err, cmux.ErrListenerClosed):
		if s.isStopped() {
			return nil
		}
	}
	return err
}

func (s *Server) isStopped() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.stopped
}

// Stop stops the server gracefully.
func (s *Server) Stop(ctx context.Context) error {
	s.stopOnce.Do(func() {
		s.stopErr = s.stop(ctx)
	})
	return s.stopErr
}

func (s *Server) stop(ctx context.Context) error {
	s.runDrain(ctx)

	s.mu.Lock()
	s.stopped = true
	listeners := s.listeners
	httpServer := s.httpServer
	grpcServer := s.grpcServer
	s.mu.Unlock()

	var errs []error

	if listeners != nil && listeners.root != nil {
		if err := listeners.root.Close(); err != nil && !errors.Is(err, net.ErrClosed) {
			errs = append(errs, err)
		}
	}

	if httpServer != nil {
		if err := httpServer.Shutdown(ctx); err != nil && !errors.Is(err, net.ErrClosed) {
			errs = append(errs, err)
			if closeErr := httpServer.Close(); closeErr != nil && !errors.Is(closeErr, net.ErrClosed) {
				errs = append(errs, closeErr)
			}
		}
	}

	if grpcServer != nil {
		done := make(chan struct{})
		go func() {
			grpcServer.GracefulStop()
			close(done)
		}()
		select {
		case <-done:
		case <-ctx.Done():
			grpcServer.Stop()
			<-done
			errs = append(errs, ctx.Err())
		}
	}

	return errors.Join(errs...)
}

func (s *Server) runDrain(ctx context.Context) {
	if s.opts.DrainFn != nil {
		s.opts.DrainFn(ctx)
	}
	if s.opts.DrainDelay <= 0 {
		return
	}
	timer := time.NewTimer(s.opts.DrainDelay)
	defer timer.Stop()
	select {
	case <-timer.C:
	case <-ctx.Done():
	}
}
