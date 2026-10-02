package server

import (
	"net/http"
	"testing"

	"github.com/dehwyy/clay/server/clayroute"
	"github.com/dehwyy/clay/server/middlewares/mwhttp"
	"github.com/go-chi/chi/v5"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"
)

type series struct {
	labels map[string]string
	value  float64
}

func handledSeries(t *testing.T, registry *prometheus.Registry) []series {
	t.Helper()
	families, err := registry.Gather()
	require.NoError(t, err)

	var result []series
	for _, family := range families {
		if family.GetName() != "http_server_handled_total" {
			continue
		}
		for _, metric := range family.GetMetric() {
			labels := make(map[string]string)
			for _, label := range metric.GetLabel() {
				labels[label.GetName()] = label.GetValue()
			}
			result = append(
				result,
				series{
					labels: labels,
					value:  metric.GetCounter().GetValue(),
				},
			)
		}
	}
	return result
}

func TestHTTPMetricsRouteLabel(t *testing.T) {
	manual := WithHTTPRoutes(
		func(r chi.Router) {
			r.Post(
				"/hook/{provider}",
				func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(http.StatusAccepted)
				},
			)
		},
	)

	tests := []struct {
		name     string
		useRoute bool
		paths    []string
		want     map[string]float64
		label    string
	}{
		{
			name:     "gateway and manual routes collapse to templates",
			useRoute: true,
			paths:    []string{"/v1/order/1", "/v1/order/2", "/v1/order/3"},
			want:     map[string]float64{"/v1/order/{id}": 3},
			label:    "http_route",
		},
		{
			name:     "manual chi route uses chi pattern",
			useRoute: true,
			paths:    []string{"/hook/a", "/hook/b"},
			want:     map[string]float64{"/hook/{provider}": 2},
			label:    "http_route",
		},
		{
			name:     "unknown paths are bounded to unmatched",
			useRoute: true,
			paths:    []string{"/nope/1", "/nope/2", "/v1/order/9"},
			want:     map[string]float64{"unmatched": 2, "/v1/order/{id}": 1},
			label:    "http_route",
		},
		{
			name:     "without the option legacy per-path series are kept",
			useRoute: false,
			paths:    []string{"/v1/order/1", "/v1/order/2"},
			want:     map[string]float64{"/v1/order/1": 1, "/v1/order/2": 1},
			label:    "http_path",
		},
	}

	for _, tt := range tests {
		t.Run(
			tt.name,
			func(t *testing.T) {
				var metricsOpts []mwhttp.ServerMetricsOption
				if tt.useRoute {
					metricsOpts = append(metricsOpts, mwhttp.WithRouteLabel(clayroute.Pattern))
				}
				metrics := mwhttp.NewServerMetrics(metricsOpts...)
				registry := prometheus.NewRegistry()
				require.NoError(t, registry.Register(metrics))

				srv := start(
					t,
					0,
					[]Option{
						WithHTTPMiddlewares(metrics.Middleware()),
						manual,
					},
					orderDesc(),
				)

				for _, path := range tt.paths {
					method := http.MethodGet
					if path[:5] == "/hook" {
						method = http.MethodPost
					}
					request, err := http.NewRequest(method, srv.httpURL(path), nil)
					require.NoError(t, err)
					response, err := http.DefaultClient.Do(request)
					require.NoError(t, err)
					require.NoError(t, response.Body.Close())
				}

				got := make(map[string]float64)
				for _, s := range handledSeries(t, registry) {
					got[s.labels[tt.label]] += s.value
				}
				require.Equal(t, tt.want, got)
			},
		)
	}
}
