package mwhttp

import (
	"net/http"
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

type serverMetricsOptions struct {
	namespace  string
	subsystem  string
	routeLabel func(*http.Request) string
}

type ServerMetricsOption func(*serverMetricsOptions)

func newServerMetricsOptions(opts ...ServerMetricsOption) *serverMetricsOptions {
	o := &serverMetricsOptions{}

	for _, opt := range opts {
		opt(o)
	}

	return o
}

// WithNamespace allows you to add a Namespace to metrics.
func WithNamespace(namespace string) ServerMetricsOption {
	return func(o *serverMetricsOptions) {
		o.namespace = namespace
	}
}

// WithSubsystem allows you to add a Subsystem to metrics.
func WithSubsystem(subsystem string) ServerMetricsOption {
	return func(o *serverMetricsOptions) {
		o.subsystem = subsystem
	}
}

func WithRouteLabel(fn func(*http.Request) string) ServerMetricsOption {
	return func(o *serverMetricsOptions) {
		o.routeLabel = fn
	}
}

// ServerMetrics represents a collection of metrics to be registered on a
// Prometheus metrics registry for a HTTP server.
type ServerMetrics struct {
	routeLabel           func(*http.Request) string
	serverStartedCounter *prometheus.CounterVec
	serverHandledCounter *prometheus.CounterVec
	// serverHandledHistogram can be nil.
	serverHandledHistogram *prometheus.HistogramVec
}

func NewServerMetrics(opts ...ServerMetricsOption) *ServerMetrics {
	serverMetricsOpts := newServerMetricsOptions(opts...)
	pathLabel := "http_path"
	startedLabels := []string{"http_method", "http_path"}
	if serverMetricsOpts.routeLabel != nil {
		pathLabel = "http_route"
		startedLabels = []string{"http_method"}
	}
	defaultLabels := []string{"http_method", pathLabel}
	defaultLabelsWithCode := []string{"http_method", pathLabel, "http_code"}

	return &ServerMetrics{
		routeLabel: serverMetricsOpts.routeLabel,
		serverStartedCounter: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: serverMetricsOpts.namespace,
				Subsystem: serverMetricsOpts.subsystem,
				Name:      "http_server_started_total",
				Help:      "Total number of requests started on the server.",
			},
			startedLabels,
		),
		serverHandledCounter: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: serverMetricsOpts.namespace,
				Subsystem: serverMetricsOpts.subsystem,
				Name:      "http_server_handled_total",
				Help:      "Total number of requests completed on the server, regardless of success or failure.",
			},
			defaultLabelsWithCode,
		),
		serverHandledHistogram: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: serverMetricsOpts.namespace,
				Subsystem: serverMetricsOpts.subsystem,
				Name:      "http_server_handling_seconds",
				Help:      "Histogram of response latency (seconds) of requests handled by the server.",
				Buckets:   []float64{0.001, 0.01, 0.1, 0.3, 0.6, 1, 3, 6, 9, 20, 30, 60, 90, 120},
			},
			defaultLabels,
		),
	}
}

// Describe sends the super-set of all possible descriptors of metrics
// collected by this Collector to the provided channel and returns once
// the last descriptor has been sent.
func (m *ServerMetrics) Describe(ch chan<- *prometheus.Desc) {
	m.serverStartedCounter.Describe(ch)
	m.serverHandledCounter.Describe(ch)
	if m.serverHandledHistogram != nil {
		m.serverHandledHistogram.Describe(ch)
	}
}

// Collect is called by the Prometheus registry when collecting
// metrics. The implementation sends each collected metric via the
// provided channel and returns once the last metric has been sent.
func (m *ServerMetrics) Collect(ch chan<- prometheus.Metric) {
	m.serverStartedCounter.Collect(ch)
	m.serverHandledCounter.Collect(ch)
	if m.serverHandledHistogram != nil {
		m.serverHandledHistogram.Collect(ch)
	}
}

func (m *ServerMetrics) Middleware() Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(
			func(w http.ResponseWriter, r *http.Request) {
				if m.routeLabel == nil {
					m.serverStartedCounter.WithLabelValues(r.Method, r.URL.Path).Inc()
				} else {
					m.serverStartedCounter.WithLabelValues(r.Method).Inc()
				}

				startedAt := time.Now()
				lwr := newLoggingResponseWriter(w)
				next.ServeHTTP(lwr, r)
				endedAt := time.Since(startedAt)

				path := r.URL.Path
				if m.routeLabel != nil {
					path = m.routeLabel(r)
				}

				m.serverHandledCounter.WithLabelValues(r.Method, path, strconv.Itoa(lwr.statusCode)).Inc()
				m.serverHandledHistogram.WithLabelValues(r.Method, path).Observe(endedAt.Seconds())
			},
		)
	}
}

// Source - https://stackoverflow.com/a/53272925
// Posted by huangapple
// Retrieved 2026-03-16, License - CC BY-SA 4.0

type loggingResponseWriter struct {
	http.ResponseWriter
	statusCode int
}

func newLoggingResponseWriter(w http.ResponseWriter) *loggingResponseWriter {
	return &loggingResponseWriter{w, http.StatusOK}
}

func (lrw *loggingResponseWriter) WriteHeader(code int) {
	lrw.statusCode = code
	lrw.ResponseWriter.WriteHeader(code)
}

func (lrw *loggingResponseWriter) Unwrap() http.ResponseWriter {
	return lrw.ResponseWriter
}
