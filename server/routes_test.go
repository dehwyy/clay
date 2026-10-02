package server

import (
	"bufio"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/require"
)

func TestHTTPRoutesWithMiddlewaresAndCustomMux(t *testing.T) {
	var (
		mu          sync.Mutex
		seenBody    string
		seenURI     string
		seenFromMux bool
	)

	readBody := func(next http.Handler) http.Handler {
		return http.HandlerFunc(
			func(w http.ResponseWriter, r *http.Request) {
				body, err := io.ReadAll(r.Body)
				if err != nil {
					http.Error(w, err.Error(), http.StatusBadRequest)
					return
				}
				r.Body = io.NopCloser(strings.NewReader(string(body)))

				mu.Lock()
				seenBody = string(body)
				seenURI = r.RequestURI
				mu.Unlock()

				w.Header().Set("X-Mw", "1")
				next.ServeHTTP(w, r)
			},
		)
	}

	preRoutedMux := chi.NewMux()
	preRoutedMux.Get(
		"/legacy",
		func(w http.ResponseWriter, r *http.Request) {
			mu.Lock()
			seenFromMux = true
			mu.Unlock()
			w.WriteHeader(http.StatusNoContent)
		},
	)

	tests := []struct {
		name    string
		opts    []Option
		request string
		status  int
		body    string
		uri     string
		fromMux bool
	}{
		{
			name: "manual route sees middleware, raw body and raw RequestURI",
			opts: []Option{
				WithHTTPMiddlewares(readBody),
				WithHTTPRoutes(
					func(r chi.Router) {
						r.Post(
							"/hook/*",
							func(w http.ResponseWriter, r *http.Request) {
								body, err := io.ReadAll(r.Body)
								if err != nil {
									http.Error(w, err.Error(), http.StatusInternalServerError)
									return
								}
								w.Write(body)
							},
						)
					},
				),
			},
			request: "POST /hook/%7euser//x?b=%20&a=1 HTTP/1.1\r\nHost: x\r\nContent-Length: 11\r\nConnection: close\r\n\r\n{\"a\": 1  }\n",
			status:  http.StatusOK,
			body:    "{\"a\": 1  }\n",
			uri:     "/hook/%7euser//x?b=%20&a=1",
		},
		{
			name: "pre-routed custom mux with middlewares does not panic",
			opts: []Option{
				WithHTTPMux(preRoutedMux),
				WithHTTPMiddlewares(readBody),
			},
			request: "GET /legacy HTTP/1.1\r\nHost: x\r\nConnection: close\r\n\r\n",
			status:  http.StatusNoContent,
			uri:     "/legacy",
			fromMux: true,
		},
		{
			name: "gateway path still served behind middlewares",
			opts: []Option{
				WithHTTPMiddlewares(readBody),
				WithHTTPRoutes(func(r chi.Router) {}),
			},
			request: "GET /v1/order/7 HTTP/1.1\r\nHost: x\r\nConnection: close\r\n\r\n",
			status:  http.StatusOK,
			uri:     "/v1/order/7",
		},
	}

	for _, tt := range tests {
		t.Run(
			tt.name,
			func(t *testing.T) {
				mu.Lock()
				seenBody, seenURI, seenFromMux = "", "", false
				mu.Unlock()

				srv := start(t, 0, tt.opts, orderDesc())

				conn, err := net.Dial("tcp", loopback(srv.srv.HTTPAddr()))
				require.NoError(t, err)
				defer conn.Close()
				require.NoError(t, conn.SetDeadline(time.Now().Add(5*time.Second)))

				_, err = conn.Write([]byte(tt.request))
				require.NoError(t, err)

				response, err := http.ReadResponse(bufio.NewReader(conn), nil)
				require.NoError(t, err)
				defer response.Body.Close()
				responseBody, err := io.ReadAll(response.Body)
				require.NoError(t, err)

				require.Equal(t, tt.status, response.StatusCode)
				require.Equal(t, "1", response.Header.Get("X-Mw"))
				require.Equal(t, tt.body, string(responseBody))

				mu.Lock()
				defer mu.Unlock()
				require.Equal(t, tt.body, seenBody)
				require.Equal(t, tt.uri, seenURI)
				require.Equal(t, tt.fromMux, seenFromMux)
			},
		)
	}
}

func TestRunWithoutDescsServesManualRoutes(t *testing.T) {
	srv := start(
		t,
		0,
		[]Option{
			WithHTTPRoutes(
				func(r chi.Router) {
					r.Get(
						"/healthz",
						func(w http.ResponseWriter, r *http.Request) {
							w.WriteHeader(http.StatusOK)
						},
					)
				},
			),
		},
	)

	response := get(t, srv.httpURL("/healthz"))
	require.Equal(t, http.StatusOK, response.StatusCode)
}
