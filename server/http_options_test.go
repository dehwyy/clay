package server

import (
	"bufio"
	"net"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestHTTPServerOptions(t *testing.T) {
	tests := []struct {
		name              string
		opts              []Option
		readHeaderTimeout time.Duration
		idleTimeout       time.Duration
		maxHeaderBytes    int
	}{
		{
			name:              "defaults",
			readHeaderTimeout: 10 * time.Second,
			idleTimeout:       120 * time.Second,
		},
		{
			name: "override timeouts",
			opts: []Option{
				WithHTTPServer(
					func(s *http.Server) {
						s.ReadHeaderTimeout = time.Second
						s.IdleTimeout = 2 * time.Second
					},
				),
			},
			readHeaderTimeout: time.Second,
			idleTimeout:       2 * time.Second,
		},
		{
			name: "several options are applied in order",
			opts: []Option{
				WithHTTPServer(func(s *http.Server) { s.MaxHeaderBytes = 4096 }),
				WithHTTPServer(func(s *http.Server) { s.ReadHeaderTimeout = 3 * time.Second }),
			},
			readHeaderTimeout: 3 * time.Second,
			idleTimeout:       120 * time.Second,
			maxHeaderBytes:    4096,
		},
		{
			name: "handler cannot be replaced",
			opts: []Option{
				WithHTTPServer(func(s *http.Server) { s.Handler = http.NotFoundHandler() }),
			},
			readHeaderTimeout: 10 * time.Second,
			idleTimeout:       120 * time.Second,
		},
	}

	for _, tt := range tests {
		t.Run(
			tt.name,
			func(t *testing.T) {
				srv := start(t, 0, tt.opts, orderDesc())

				require.Equal(t, tt.readHeaderTimeout, srv.srv.httpServer.ReadHeaderTimeout)
				require.Equal(t, tt.idleTimeout, srv.srv.httpServer.IdleTimeout)
				require.Equal(t, tt.maxHeaderBytes, srv.srv.httpServer.MaxHeaderBytes)

				response := get(t, srv.httpURL("/v1/order/1"))
				require.Equal(t, http.StatusOK, response.StatusCode)
			},
		)
	}
}

func TestReadHeaderTimeoutDropsSlowClient(t *testing.T) {
	srv := start(
		t,
		0,
		[]Option{
			WithHTTPServer(func(s *http.Server) { s.ReadHeaderTimeout = 200 * time.Millisecond }),
		},
		orderDesc(),
	)

	conn, err := net.Dial("tcp", loopback(srv.srv.HTTPAddr()))
	require.NoError(t, err)
	defer conn.Close()

	_, err = conn.Write([]byte("GET /v1/order/1 HTTP/1.1\r\nHost: x\r\n"))
	require.NoError(t, err)

	require.NoError(t, conn.SetReadDeadline(time.Now().Add(3*time.Second)))
	response, err := http.ReadResponse(bufio.NewReader(conn), nil)
	if err == nil {
		defer response.Body.Close()
		require.Equal(t, http.StatusRequestTimeout, response.StatusCode)
		return
	}
	require.NotErrorIs(t, err, os.ErrDeadlineExceeded)
}
