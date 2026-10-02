package server

import (
	"context"
	"net"
	"net/http"
	"strconv"
	"testing"
	"time"

	"github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
	"github.com/not-for-prod/clay/transport"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
)

type fakeDesc struct {
	http func(mux *runtime.ServeMux) error
}

func (d *fakeDesc) RegisterGRPC(g *grpc.Server) {
	healthpb.RegisterHealthServer(g, health.NewServer())
}

func (d *fakeDesc) RegisterHTTP(_ context.Context, mux *runtime.ServeMux) error {
	if d.http == nil {
		return nil
	}
	return d.http(mux)
}

func (d *fakeDesc) SwaggerDef() []byte {
	return []byte(`{"swagger":"2.0","paths":{}}`)
}

func orderDesc() *fakeDesc {
	return &fakeDesc{
		http: func(mux *runtime.ServeMux) error {
			return mux.HandlePath(
				http.MethodGet,
				"/v1/order/{id}",
				func(w http.ResponseWriter, r *http.Request, _ map[string]string) {
					_, err := runtime.AnnotateContext(
						r.Context(),
						mux,
						r,
						"/test.Orders/Get",
						runtime.WithHTTPPathPattern("/v1/order/{id}"),
					)
					if err != nil {
						http.Error(w, err.Error(), http.StatusInternalServerError)
						return
					}
					w.WriteHeader(http.StatusOK)
				},
			)
		},
	}
}

func freePort(t *testing.T) int {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	port := listener.Addr().(*net.TCPAddr).Port
	require.NoError(t, listener.Close())
	return port
}

type started struct {
	srv  *Server
	done chan error
}

func start(t *testing.T, port int, opts []Option, descs ...*fakeDesc) *started {
	t.Helper()
	srv := NewServer(port, opts...)
	done := make(chan error, 1)

	serviceDescs := make([]transport.ServiceDesc, 0, len(descs))
	for _, desc := range descs {
		serviceDescs = append(serviceDescs, desc)
	}

	go func() {
		done <- srv.Run(serviceDescs...)
	}()

	select {
	case <-srv.Ready():
	case err := <-done:
		require.FailNow(t, "server stopped before ready", "%v", err)
	case <-time.After(5 * time.Second):
		require.FailNow(t, "server not ready")
	}

	t.Cleanup(
		func() {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			if err := srv.Stop(ctx); err != nil {
				t.Logf("cleanup stop: %v", err)
			}
		},
	)

	return &started{srv: srv, done: done}
}

func (s *started) httpURL(path string) string {
	return "http://" + loopback(s.srv.HTTPAddr()) + path
}

func (s *started) waitRun(t *testing.T) error {
	t.Helper()
	select {
	case err := <-s.done:
		return err
	case <-time.After(5 * time.Second):
		require.FailNow(t, "Run did not return")
		return nil
	}
}

func loopback(addr net.Addr) string {
	tcp := addr.(*net.TCPAddr)
	return net.JoinHostPort("127.0.0.1", strconv.Itoa(tcp.Port))
}

func get(t *testing.T, url string) *http.Response {
	t.Helper()
	response, err := http.Get(url)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, response.Body.Close()) })
	return response
}
