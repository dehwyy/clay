package server

import (
	"context"
	"net"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestRunReturnsNilAfterStop(t *testing.T) {
	tests := []struct {
		name string
		port func(t *testing.T) (int, []Option)
	}{
		{
			name: "single port",
			port: func(t *testing.T) (int, []Option) {
				return 0, nil
			},
		},
		{
			name: "separate ports",
			port: func(t *testing.T) (int, []Option) {
				return freePort(t), []Option{WithHTTPPort(freePort(t))}
			},
		},
		{
			name: "grpc disabled",
			port: func(t *testing.T) (int, []Option) {
				return freePort(t), []Option{WithHTTPPort(freePort(t)), WithGRPCDisabled()}
			},
		},
	}

	for _, tt := range tests {
		t.Run(
			tt.name,
			func(t *testing.T) {
				port, opts := tt.port(t)
				srv := start(t, port, opts, orderDesc())

				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()

				require.NoError(t, srv.srv.Stop(ctx))
				require.NoError(t, srv.waitRun(t))
				require.NoError(t, srv.srv.Stop(ctx))

				_, err := net.DialTimeout("tcp", loopback(srv.srv.HTTPAddr()), time.Second)
				require.Error(t, err)
			},
		)
	}
}

func TestStopBeforeRunIsNoop(t *testing.T) {
	srv := NewServer(0)

	require.NoError(t, srv.Stop(context.Background()))
	require.NoError(t, srv.Run(orderDesc()))
}

func TestDrain(t *testing.T) {
	tests := []struct {
		name  string
		delay time.Duration
	}{
		{name: "no delay"},
		{name: "with delay", delay: 150 * time.Millisecond},
	}

	for _, tt := range tests {
		t.Run(
			tt.name,
			func(t *testing.T) {
				var (
					mu           sync.Mutex
					calls        int
					statusInHook int
				)

				var srv *started
				srv = start(
					t,
					0,
					[]Option{
						WithDrain(
							func(ctx context.Context) {
								response, err := http.Get(srv.httpURL("/v1/order/1"))
								mu.Lock()
								defer mu.Unlock()
								calls++
								if err == nil {
									statusInHook = response.StatusCode
									response.Body.Close()
								}
							},
							tt.delay,
						),
					},
					orderDesc(),
				)

				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()

				startedAt := time.Now()
				require.NoError(t, srv.srv.Stop(ctx))
				elapsed := time.Since(startedAt)

				mu.Lock()
				defer mu.Unlock()
				require.Equal(t, 1, calls)
				require.Equal(t, http.StatusOK, statusInHook)
				require.GreaterOrEqual(t, elapsed, tt.delay)

				_, err := net.DialTimeout("tcp", loopback(srv.srv.HTTPAddr()), time.Second)
				require.Error(t, err)
			},
		)
	}
}

func TestDrainDelayHonoursContext(t *testing.T) {
	srv := start(
		t,
		0,
		[]Option{WithDrain(func(context.Context) {}, time.Minute)},
		orderDesc(),
	)

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	startedAt := time.Now()
	require.Error(t, srv.srv.Stop(ctx))
	require.Less(t, time.Since(startedAt), 5*time.Second)
}
