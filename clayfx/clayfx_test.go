package clayfx_test

import (
	"context"
	"net"
	"net/http"
	"strconv"
	"testing"
	"time"

	"github.com/dehwyy/clay/clayfx"
	"github.com/dehwyy/clay/server"
	"github.com/go-chi/chi/v5"
	"github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
	"github.com/stretchr/testify/require"
	"go.uber.org/fx"
	"go.uber.org/fx/fxtest"
	"google.golang.org/grpc"
)

type desc struct {
	registered *bool
}

func (d *desc) RegisterGRPC(*grpc.Server) {
	*d.registered = true
}

func (d *desc) RegisterHTTP(context.Context, *runtime.ServeMux) error {
	return nil
}

func (d *desc) SwaggerDef() []byte {
	return []byte(`{"swagger":"2.0","paths":{}}`)
}

func healthRoutes() server.Option {
	return server.WithHTTPRoutes(
		func(r chi.Router) {
			r.Get(
				"/healthz",
				func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(http.StatusOK)
				},
			)
		},
	)
}

func healthz(t *testing.T, srv *server.Server) int {
	t.Helper()
	tcp := srv.HTTPAddr().(*net.TCPAddr)
	response, err := http.Get("http://127.0.0.1:" + strconv.Itoa(tcp.Port) + "/healthz")
	require.NoError(t, err)
	defer response.Body.Close()
	return response.StatusCode
}

func TestModuleLifecycle(t *testing.T) {
	registered := false

	tests := []struct {
		name string
		opt  func() fx.Option
	}{
		{
			name: "static config",
			opt: func() fx.Option {
				return clayfx.Module(0, healthRoutes())
			},
		},
		{
			name: "config from graph",
			opt: func() fx.Option {
				return fx.Options(
					fx.Supply(0),
					clayfx.ModuleFromConfig(
						func(port int) clayfx.Config {
							return clayfx.Config{
								RPCPort: port,
								Options: []server.Option{healthRoutes()},
							}
						},
					),
				)
			},
		},
	}

	for _, tt := range tests {
		t.Run(
			tt.name,
			func(t *testing.T) {
				registered = false
				var srv *server.Server

				app := fxtest.New(
					t,
					tt.opt(),
					clayfx.ProvideDesc(func() *desc { return &desc{registered: &registered} }),
					fx.Populate(&srv),
				)

				app.RequireStart()
				require.True(t, registered)
				require.Equal(t, http.StatusOK, healthz(t, srv))

				port := srv.HTTPAddr().(*net.TCPAddr).Port
				app.RequireStop()

				_, err := net.DialTimeout("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)), time.Second)
				require.Error(t, err)
			},
		)
	}
}

func TestModuleWithoutDescs(t *testing.T) {
	var srv *server.Server

	app := fxtest.New(
		t,
		clayfx.Module(0, healthRoutes()),
		fx.Populate(&srv),
	)

	app.RequireStart()
	defer app.RequireStop()

	require.Equal(t, http.StatusOK, healthz(t, srv))
}

func TestModuleStartFailureIsReported(t *testing.T) {
	app := fx.New(
		fx.NopLogger,
		clayfx.Module(0, server.WithGRPCListenHost("127.0.0.1")),
	)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err := app.Start(ctx)
	require.ErrorContains(t, err, "gRPC listen host")
}
