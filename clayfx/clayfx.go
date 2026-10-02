package clayfx

import (
	"context"
	"errors"

	"github.com/not-for-prod/clay/server"
	"github.com/not-for-prod/clay/transport"
	"go.uber.org/fx"
)

const DescsGroup = "clay.descs"

type Config struct {
	RPCPort int
	Options []server.Option
}

type Descs struct {
	fx.In

	Items []transport.ServiceDesc `group:"clay.descs"`
}

func Module(rpcPort int, opts ...server.Option) fx.Option {
	return fx.Options(
		fx.Supply(
			Config{
				RPCPort: rpcPort,
				Options: opts,
			},
		),
		core(),
	)
}

func ModuleFromConfig(newConfig any) fx.Option {
	return fx.Options(
		fx.Provide(newConfig),
		core(),
	)
}

func ProvideDesc(constructor any) fx.Option {
	return fx.Provide(
		fx.Annotate(
			constructor,
			fx.As(new(transport.ServiceDesc)),
			fx.ResultTags(`group:"clay.descs"`),
		),
	)
}

func core() fx.Option {
	return fx.Options(
		fx.Provide(newServer),
		fx.Invoke(register),
	)
}

func newServer(config Config) *server.Server {
	return server.NewServer(
		config.RPCPort,
		config.Options...,
	)
}

type registerParams struct {
	fx.In

	Lifecycle  fx.Lifecycle
	Shutdowner fx.Shutdowner
	Server     *server.Server
	Descs      Descs
}

func register(params registerParams) {
	done := make(chan error, 1)

	params.Lifecycle.Append(
		fx.Hook{
			OnStart: func(ctx context.Context) error {
				go func() {
					err := params.Server.Run(params.Descs.Items...)
					if err != nil {
						err = errors.Join(
							err,
							params.Shutdowner.Shutdown(fx.ExitCode(1)),
						)
					}
					done <- err
				}()

				select {
				case <-params.Server.Ready():
					return nil
				case err := <-done:
					if err == nil {
						return errors.New("clay server stopped before it became ready")
					}
					return err
				case <-ctx.Done():
					return ctx.Err()
				}
			},
			OnStop: func(ctx context.Context) error {
				stopErr := params.Server.Stop(ctx)

				select {
				case runErr := <-done:
					return errors.Join(
						stopErr,
						runErr,
					)
				case <-ctx.Done():
					return errors.Join(
						stopErr,
						ctx.Err(),
					)
				}
			},
		},
	)
}
