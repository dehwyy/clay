package server

import (
	"net"
	"strconv"
	"time"

	"github.com/pkg/errors"
	"github.com/soheilhy/cmux"
)

const (
	listenRetryWait     = 500 * time.Millisecond
	listenRetryDuration = 10 * time.Second
)

type listenerSet struct {
	mainListener cmux.CMux
	root         net.Listener
	HTTP         net.Listener
	GRPC         net.Listener
}

func (l *listenerSet) closeAll() {
	for _, listener := range []net.Listener{l.root, l.HTTP, l.GRPC} {
		if listener != nil {
			_ = listener.Close()
		}
	}
}

func (s *Server) initListeners() error {
	liSet := &listenerSet{}
	shared := s.opts.RPCPort == s.opts.HTTPPort && !s.opts.GRPCDisabled

	if shared && s.opts.GRPCListenHost != "" {
		return errors.New("gRPC listen host requires a gRPC port different from the HTTP port")
	}

	var err error
	switch {
	case s.opts.GRPCDisabled:
		liSet.HTTP, err = newListener("", s.opts.HTTPPort)
	case shared:
		liSet.root, err = newListener("", s.opts.RPCPort)
		if err == nil {
			mux := cmux.New(liSet.root)
			liSet.GRPC = mux.Match(cmux.HTTP2())
			liSet.HTTP = mux.Match(cmux.Any())
			liSet.mainListener = mux
		}
	default:
		liSet.GRPC, err = newListener(s.opts.GRPCListenHost, s.opts.RPCPort)
		if err == nil {
			liSet.HTTP, err = newListener("", s.opts.HTTPPort)
		}
	}
	if err != nil {
		liSet.closeAll()
		return errors.Wrap(err, "couldn't create listeners")
	}

	s.listeners = liSet

	return nil
}

// newListener start net.Listener on a host and port.
// It keeps retrying if port is already in use.
func newListener(host string, port int) (net.Listener, error) {
	var listener net.Listener
	var err error
	start := time.Now()
	for time.Since(start) < listenRetryDuration {
		listener, err = net.Listen("tcp", net.JoinHostPort(host, strconv.Itoa(port)))
		if err == nil {
			return listener, nil
		}
		time.Sleep(listenRetryWait)
	}
	return nil, err
}
