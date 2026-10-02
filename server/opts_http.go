package server

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
)

const (
	defaultReadHeaderTimeout = 10 * time.Second
	defaultIdleTimeout       = 120 * time.Second
)

func WithHTTPServer(fn func(*http.Server)) Option {
	return func(o *serverOpts) {
		o.HTTPServerFns = append(o.HTTPServerFns, fn)
	}
}

func WithHTTPRoutes(fn func(r chi.Router)) Option {
	return func(o *serverOpts) {
		o.HTTPRoutes = append(o.HTTPRoutes, fn)
	}
}
