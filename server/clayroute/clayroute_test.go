package clayroute

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/require"
)

func TestPattern(t *testing.T) {
	tests := []struct {
		name       string
		withMw     bool
		method     string
		path       string
		wantStatus int
		want       string
	}{
		{name: "template of chi route", withMw: true, method: http.MethodGet, path: "/orders/42", wantStatus: http.StatusOK, want: "/orders/{id}"},
		{name: "template of nested chi route", withMw: true, method: http.MethodPost, path: "/hook/psp", wantStatus: http.StatusAccepted, want: "/hook/{provider}"},
		{name: "unknown path is unmatched", withMw: true, method: http.MethodGet, path: "/missing", wantStatus: http.StatusNotFound, want: Unmatched},
		{name: "chi pattern without middleware", withMw: false, method: http.MethodGet, path: "/orders/42", wantStatus: http.StatusOK, want: "/orders/{id}"},
	}

	for _, tt := range tests {
		t.Run(
			tt.name,
			func(t *testing.T) {
				var got string
				capture := func(next http.Handler) http.Handler {
					return http.HandlerFunc(
						func(w http.ResponseWriter, r *http.Request) {
							next.ServeHTTP(w, r)
							got = Pattern(r)
						},
					)
				}

				root := chi.NewMux()
				if tt.withMw {
					root.Use(Middleware())
				}
				root.Use(capture)

				inner := chi.NewMux()
				inner.Get("/orders/{id}", func(w http.ResponseWriter, r *http.Request) {})
				inner.Route(
					"/hook",
					func(r chi.Router) {
						r.Post(
							"/{provider}",
							func(w http.ResponseWriter, r *http.Request) {
								w.WriteHeader(http.StatusAccepted)
							},
						)
					},
				)
				root.Mount("/", inner)

				recorder := httptest.NewRecorder()
				root.ServeHTTP(recorder, httptest.NewRequest(tt.method, tt.path, nil))

				require.Equal(t, tt.wantStatus, recorder.Code)
				require.Equal(t, tt.want, got)
			},
		)
	}
}

func TestPatternWithoutChiContext(t *testing.T) {
	require.Equal(t, Unmatched, Pattern(httptest.NewRequest(http.MethodGet, "/x", nil)))
}
