package tunnel

import (
	"context"
	"io"
	"os"

	"github.com/benice2me11/codexify-go/internal/config"
	"github.com/benice2me11/codexify-go/internal/supervisor"
)

type Factory struct {
	Config  config.Config
	Output  io.Writer
	Watcher *PollWatcher
}

func (f *Factory) Start(ctx context.Context) (supervisor.Process, error) {
	_ = os.Remove(f.Config.Tunnel.HealthURLFile)
	if f.Watcher != nil {
		f.Watcher.Reset()
	}
	executable, err := ResolveExecutable(ctx, f.Config.Tunnel)
	if err != nil {
		return nil, err
	}
	output := f.Output
	if f.Watcher != nil {
		output = io.MultiWriter(output, f.Watcher)
	}
	cf := supervisor.CommandFactory{
		Spec: supervisor.CommandSpec{
			Path: executable,
			Args: f.Config.TunnelArgs(),
			Env:  f.Config.Tunnel.Environment,
		},
		Output: output,
	}
	return cf.Start(ctx)
}
