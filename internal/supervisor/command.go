package supervisor

import (
	"context"
	"io"
	"os"
	"os/exec"
	"sync"
)

type CommandSpec struct {
	Path string
	Args []string
	Env  map[string]string
}

type CommandFactory struct {
	Spec   CommandSpec
	Output io.Writer
}

func (f *CommandFactory) Start(context.Context) (Process, error) {
	cmd := exec.Command(f.Spec.Path, f.Spec.Args...)
	cmd.Env = append([]string{}, os.Environ()...)
	for k, v := range f.Spec.Env {
		cmd.Env = append(cmd.Env, k+"="+v)
	}
	if f.Output != nil {
		cmd.Stdout = f.Output
		cmd.Stderr = f.Output
	}
	configureProcess(cmd)
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	releaseProcess, err := attachProcess(cmd)
	if err != nil {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		return nil, err
	}
	p := &commandProcess{
		cmd:            cmd,
		done:           make(chan error, 1),
		releaseProcess: releaseProcess,
	}
	go func() {
		err := cmd.Wait()
		p.release()
		p.done <- err
		close(p.done)
	}()
	return p, nil
}

type commandProcess struct {
	cmd            *exec.Cmd
	done           chan error
	stopOnce       sync.Once
	releaseOnce    sync.Once
	releaseProcess func()
}

func (p *commandProcess) release() {
	p.releaseOnce.Do(func() {
		if p.releaseProcess != nil {
			p.releaseProcess()
		}
	})
}

func (p *commandProcess) PID() int {
	if p.cmd.Process == nil {
		return 0
	}
	return p.cmd.Process.Pid
}

func (p *commandProcess) Done() <-chan error { return p.done }

func (p *commandProcess) Stop(ctx context.Context) error {
	var stopErr error
	p.stopOnce.Do(func() {
		stopErr = stopProcessTree(ctx, p.PID())
		if stopErr != nil {
			stopErr = forceKillProcessTree(p.PID())
		}
	})
	if stopErr != nil {
		return stopErr
	}
	select {
	case <-p.done:
		if err := waitProcessTreeExit(ctx, p.PID()); err == nil {
			return nil
		} else {
			_ = forceKillProcessTree(p.PID())
			if ctx.Err() != nil {
				return ctx.Err()
			}
			return err
		}
	case <-ctx.Done():
		_ = forceKillProcessTree(p.PID())
		select {
		case <-p.done:
		default:
		}
		return ctx.Err()
	}
}
