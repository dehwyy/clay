package clayroute

import (
	"context"
	"net/http"
	"sync"

	"github.com/go-chi/chi/v5"
	"github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
	"google.golang.org/grpc/metadata"
)

const Unmatched = "unmatched"

type holderKey struct{}

type holder struct {
	mu      sync.Mutex
	pattern string
}

func (h *holder) set(pattern string) {
	h.mu.Lock()
	h.pattern = pattern
	h.mu.Unlock()
}

func (h *holder) get() string {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.pattern
}

func Middleware() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(
			func(w http.ResponseWriter, r *http.Request) {
				ctx := context.WithValue(
					r.Context(),
					holderKey{},
					&holder{},
				)
				next.ServeHTTP(
					w,
					r.WithContext(ctx),
				)
			},
		)
	}
}

func GatewayOption() runtime.ServeMuxOption {
	return runtime.WithMetadata(
		func(ctx context.Context, r *http.Request) metadata.MD {
			pattern, ok := runtime.HTTPPathPattern(ctx)
			if !ok {
				return nil
			}
			h, ok := r.Context().Value(holderKey{}).(*holder)
			if !ok {
				return nil
			}
			h.set(pattern)
			return nil
		},
	)
}

func Pattern(r *http.Request) string {
	if h, ok := r.Context().Value(holderKey{}).(*holder); ok {
		if pattern := h.get(); pattern != "" {
			return pattern
		}
	}
	rctx := chi.RouteContext(r.Context())
	if rctx == nil {
		return Unmatched
	}
	pattern := rctx.RoutePattern()
	if pattern == "" || pattern == "/*" || pattern == "/" && len(rctx.RoutePatterns) > 1 {
		return Unmatched
	}
	return pattern
}
