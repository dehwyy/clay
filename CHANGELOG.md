# Changelog

## v0.5.0 (proposed)

Backward compatible with v0.4.x at the API level. Behavior changes are listed below.

### Added
- `server.WithHTTPServer(fn)`: customize `http.Server` (ReadHeaderTimeout, IdleTimeout, MaxHeaderBytes, ErrorLog, ...).
- `server.WithHTTPRoutes(fn)`: manual routes (webhooks, file downloads, raw body for signature checks).
  Mounted after HTTP middlewares and before the gateway catch-all.
- `server.WithDrain(fn, delay)`: `Stop` calls `fn(ctx)`, waits `delay`, then shuts down.
- `server.WithGRPCListenHost(host)`: bind gRPC to e.g. `127.0.0.1`.
- `server.WithGRPCDisabled()`: no gRPC listener, HTTP only.
- `server.WithReflection(bool)`: gRPC reflection toggle, enabled by default as before.
- `server.WithGRPCTracing(interceptor)`: unary interceptor that always runs first.
- `Server.Ready()`, `Server.HTTPAddr()`, `Server.GRPCAddr()`.
- `server/clayroute`: `Middleware()`, `GatewayOption()`, `Pattern(r)`, `Unmatched`.
- `mwhttp.WithRouteLabel(fn)`: metrics labelled by `http_route` instead of `http_path`.
- `clayfx`: `Module`, `ModuleFromConfig`, `ProvideDesc`, `Descs`, `Config`.
- `Run()` without descriptors serves only manual routes and swagger (health-only services).

### Changed
- `http.Server` gets `ReadHeaderTimeout` 10s and `IdleTimeout` 120s by default (v0.4: none). Override via `WithHTTPServer`.
- `Run` returns `nil` after `Stop` (v0.4 returned `http.ErrServerClosed`).
- `Stop` closes the cmux root listener, force-closes HTTP when ctx expires and falls back from `GracefulStop` to `Stop` on `ctx.Done()`. Repeated calls return the first result.
- `WithGRPCUnaryMiddlewares` / `WithGRPCMiddlewares` accumulate and chain instead of panicking on the second call.
- HTTP middlewares are mounted on an outer router, so `WithHTTPMux` with pre-registered routes plus `WithHTTPMiddlewares` no longer panics.
- `mwhttp` response writer implements `Unwrap`.

### Unchanged by design
- `WithHTTPMiddlewares` still replaces the previous list.
- Without `WithRouteLabel`, HTTP metrics keep the `http_path` label.
- A gRPC port equal to the HTTP port still means one cmux listener; `WithGRPCListenHost` then makes `Run` fail.
