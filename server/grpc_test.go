package server

import (
	"context"
	"net"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
)

func recorder(name string, calls *[]string) grpc.UnaryServerInterceptor {
	return func(
		ctx context.Context,
		req any,
		_ *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
	) (any, error) {
		*calls = append(*calls, name)
		return handler(ctx, req)
	}
}

func checkHealth(t *testing.T, addr string) error {
	t.Helper()
	conn, err := grpc.NewClient(
		addr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	require.NoError(t, err)
	defer conn.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	_, err = healthpb.NewHealthClient(conn).Check(ctx, &healthpb.HealthCheckRequest{})
	return err
}

func TestGRPCInterceptorOrder(t *testing.T) {
	tests := []struct {
		name string
		opts func(calls *[]string) []Option
		want []string
	}{
		{
			name: "tracing first even when declared last",
			opts: func(calls *[]string) []Option {
				return []Option{
					WithGRPCUnaryMiddlewares(recorder("a", calls), recorder("b", calls)),
					WithGRPCTracing(recorder("tracer", calls)),
				}
			},
			want: []string{"tracer", "a", "b"},
		},
		{
			name: "repeated middleware options are chained, no panic",
			opts: func(calls *[]string) []Option {
				return []Option{
					WithGRPCUnaryMiddlewares(recorder("a", calls)),
					WithGRPCMiddlewares(recorder("b", calls)),
					WithGRPCUnaryMiddlewares(recorder("c", calls)),
				}
			},
			want: []string{"a", "b", "c"},
		},
		{
			name: "tracing only",
			opts: func(calls *[]string) []Option {
				return []Option{WithGRPCTracing(recorder("tracer", calls))}
			},
			want: []string{"tracer"},
		},
	}

	for _, tt := range tests {
		t.Run(
			tt.name,
			func(t *testing.T) {
				var calls []string
				srv := start(t, 0, tt.opts(&calls), orderDesc())

				require.NoError(t, checkHealth(t, loopback(srv.srv.GRPCAddr())))
				require.Equal(t, tt.want, calls)
			},
		)
	}
}

func nonLoopbackIP(t *testing.T) string {
	t.Helper()
	addrs, err := net.InterfaceAddrs()
	require.NoError(t, err)
	for _, addr := range addrs {
		ipNet, ok := addr.(*net.IPNet)
		if ok && ipNet.IP.To4() != nil && !ipNet.IP.IsLoopback() {
			return ipNet.IP.String()
		}
	}
	return ""
}

func TestGRPCListenHost(t *testing.T) {
	external := nonLoopbackIP(t)

	tests := []struct {
		name             string
		host             string
		wantExternalOpen bool
	}{
		{name: "all interfaces by default", host: "", wantExternalOpen: true},
		{name: "loopback only", host: "127.0.0.1", wantExternalOpen: false},
	}

	for _, tt := range tests {
		t.Run(
			tt.name,
			func(t *testing.T) {
				if external == "" {
					t.Skip("no non-loopback interface")
				}

				grpcPort := freePort(t)
				srv := start(
					t,
					grpcPort,
					[]Option{
						WithHTTPPort(freePort(t)),
						WithGRPCListenHost(tt.host),
					},
					orderDesc(),
				)

				require.NoError(t, checkHealth(t, net.JoinHostPort("127.0.0.1", strconv.Itoa(grpcPort))))

				conn, err := net.DialTimeout(
					"tcp",
					net.JoinHostPort(external, strconv.Itoa(grpcPort)),
					time.Second,
				)
				if conn != nil {
					conn.Close()
				}
				if tt.wantExternalOpen {
					require.NoError(t, err)
				} else {
					require.Error(t, err)
				}
				require.NotNil(t, srv.srv.GRPCAddr())
			},
		)
	}
}

func TestGRPCListenHostOnSharedPortFails(t *testing.T) {
	srv := NewServer(freePort(t), WithGRPCListenHost("127.0.0.1"))

	err := srv.Run(orderDesc())
	require.ErrorContains(t, err, "gRPC listen host")
}

func TestGRPCDisabled(t *testing.T) {
	grpcPort := freePort(t)
	srv := start(
		t,
		grpcPort,
		[]Option{WithHTTPPort(freePort(t)), WithGRPCDisabled()},
		orderDesc(),
	)

	require.Nil(t, srv.srv.GRPCAddr())

	_, err := net.DialTimeout("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(grpcPort)), time.Second)
	require.Error(t, err)

	response := get(t, srv.httpURL("/v1/order/1"))
	require.Equal(t, 200, response.StatusCode)
}

func TestReflection(t *testing.T) {
	tests := []struct {
		name string
		opts []Option
		want bool
	}{
		{name: "enabled by default", want: true},
		{name: "explicitly disabled", opts: []Option{WithReflection(false)}, want: false},
		{name: "explicitly enabled", opts: []Option{WithReflection(true)}, want: true},
	}

	for _, tt := range tests {
		t.Run(
			tt.name,
			func(t *testing.T) {
				srv := start(t, 0, tt.opts, orderDesc())

				_, ok := srv.srv.grpcServer.GetServiceInfo()["grpc.reflection.v1.ServerReflection"]
				require.Equal(t, tt.want, ok)
			},
		)
	}
}
