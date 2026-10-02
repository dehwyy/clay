# clay

[![Build Status](https://travis-ci.org/utrack/clay.svg?branch=master)](https://travis-ci.org/utrack/clay)

Minimal server platform for gRPC and REST+Swagger APIs in Go

Using clay you can automatically spin up HTTP handlers for your gRPC server with
complete Swagger defs with a few lines of code.

This project provides the HTTP+Swagger handler generator and optional server that you
can use to serve your handlers via any protocol.

# protobuf generator compatibility

clay/v3 uses new protobuf generator [google.golang.org/protobuf](https://pkg.go.dev/mod/google.golang.org/protobuf).
If you're using old generator ([golang/protobuf](https://github.com/golang/protobuf) and protoc-gen-go <v1.20.0) then consider using `clay/v2` instead.

Read more about this migration here: https://blog.golang.org/protobuf-apiv2 .

# Migration from v2

Migration from v2 to v3 is intended to be as straightforward as possible - just replace generator v2 with v3 and you're good to go.
Don't forget to change your protobuf generator as well - see previous section for details.

NB: to use new protobuf generator, you need to change `go_package` directives to be an absolute module path (i.e. `github.com/foo/bar/baz` instead of `baz`).

## Requirements

Since new [Semantic Import Versioning](https://research.swtch.com/vgo-import) is used, you are required to
use [Go1.10.3+](https://golang.org/doc/devel/release.html#go1.10)

## How?

Check out an [example server](https://github.com/utrack/clay/wiki/Build-and-run-an-example-SummatorService-using-clay-Server)
for a quick start if you're experienced with gRPC, or dive into [step-by-step docs](https://github.com/utrack/clay/wiki/Describe-and-create-your-own-API)
for a full guide.

## Contributing

You may contribute in several ways like creating new features, fixing bugs,
improving documentation and/or examples using GitHub pull requests.

# Server options (v0.5.0)

```go
srv := server.NewServer(
	grpcPort,
	server.WithHTTPPort(httpPort),
	server.WithGRPCListenHost("127.0.0.1"),
	server.WithHTTPServer(func(s *http.Server) { s.ReadHeaderTimeout = 5 * time.Second }),
	server.WithHTTPMiddlewares(metrics.Middleware(), signature.Middleware()),
	server.WithHTTPRoutes(func(r chi.Router) { r.Post("/hook/{provider}", webhook) }),
	server.WithGRPCTracing(tracing),
	server.WithGRPCUnaryMiddlewares(recoverer, auth),
	server.WithDrain(readiness.SetNotReady, 5*time.Second),
	server.WithRuntimeServeMuxOpts(runtime.WithErrorHandler(handleErr)),
)
```

- Defaults: `ReadHeaderTimeout` 10s, `IdleTimeout` 120s. `WithHTTPServer` runs after defaults and cannot replace the handler.
- `WithHTTPRoutes` routes sit behind HTTP middlewares and win over gateway paths. Middlewares see the original `RequestURI` and the unread body, so a signature middleware reads `io.ReadAll(r.Body)` and puts back `io.NopCloser`.
- Order: middlewares of a custom `WithHTTPMux` run first, then `WithHTTPMiddlewares`, then routes. Routes registered directly on the custom mux bypass `WithHTTPMiddlewares`.
- `WithHTTPMiddlewares` replaces the list on each call: pass all middlewares in one call. Put metrics outermost.
- gRPC unary middlewares: repeated calls accumulate. `WithGRPCTracing` runs first whatever the option order.
- Closed gRPC: use a dedicated gRPC port with `WithGRPCListenHost("127.0.0.1")`, or `WithGRPCDisabled()`. With one shared port (default) the host option makes `Run` fail, because the HTTP traffic would be loopback-only too.
- Shutdown: `Stop(ctx)` runs the drain hook, waits the delay, stops accepting, shuts HTTP down and stops gRPC gracefully. When ctx ends first, remaining work is cut. `Run` returns `nil` after `Stop`.
- `Ready()` is closed once the listeners are bound; `HTTPAddr()` and `GRPCAddr()` are valid after that (useful with port 0 in tests).
- `Run()` with no descriptors serves manual routes only.

## Metrics by route template

```go
metrics := mwhttp.NewServerMetrics(
	mwhttp.WithNamespace("app"),
	mwhttp.WithRouteLabel(clayroute.Pattern),
)
```

Labels become `http_route` (gateway template such as `/v1/order/{id}`, chi pattern for manual routes, `unmatched` otherwise). The started counter carries only `http_method`, because the route is known after the request is handled. Without the option the old `http_path` label is kept.
`server` installs `clayroute.Middleware()` and `clayroute.GatewayOption()` itself.

## fx

```go
fx.New(
	clayfx.Module(grpcPort, server.WithHTTPPort(httpPort)),
	clayfx.ProvideDesc(orderv1.NewOrderServiceDesc),
).Run()
```

`Module` starts `Run` on OnStart (startup failures fail the app, later failures call `fx.Shutdowner`) and calls `Stop` on OnStop. For a config from the graph use `clayfx.ModuleFromConfig(func(cfg *Config) clayfx.Config {...})`. Descriptors are collected from the `clay.descs` group.
