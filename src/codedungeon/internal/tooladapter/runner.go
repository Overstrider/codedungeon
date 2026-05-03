package tooladapter

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/loldinis/codedungeon/internal/osadapter"
)

type SystemRunner struct {
	Adapter osadapter.Adapter
}

func NewSystemRunner() SystemRunner {
	return SystemRunner{Adapter: osadapter.Detect()}
}

func (r SystemRunner) Run(ctx context.Context, req Command) (CommandResult, error) {
	if req.Name == "" {
		return CommandResult{}, ToolError{Kind: ErrorInvalid, Operation: "command", Err: errors.New("command name is required")}
	}
	stdctx := ctx
	if stdctx == nil {
		stdctx = context.Background()
	}
	if req.Timeout > 0 {
		var cancel context.CancelFunc
		stdctx, cancel = context.WithTimeout(stdctx, req.Timeout)
		defer cancel()
	}
	name, err := r.resolve(req.Name)
	if err != nil {
		return CommandResult{}, ToolError{Kind: ErrorNotFound, Tool: req.Name, Operation: operation(req.Name, req.Args), Err: err}
	}
	started := time.Now()
	cmd := exec.Command(name, req.Args...)
	prepareCommandForTreeKill(cmd)
	if req.Dir != "" {
		cmd.Dir = req.Dir
	}
	if len(req.Env) > 0 {
		cmd.Env = append(os.Environ(), req.Env...)
	}
	if req.Stdin != "" {
		cmd.Stdin = strings.NewReader(req.Stdin)
	}
	var stdout, stderr bytes.Buffer
	var outputMu sync.Mutex
	if req.Stdout != nil {
		cmd.Stdout = lockingWriter{w: req.Stdout, mu: &outputMu}
	} else {
		cmd.Stdout = lockingWriter{w: &stdout, mu: &outputMu}
	}
	if req.Stderr != nil {
		cmd.Stderr = lockingWriter{w: req.Stderr, mu: &outputMu}
	} else {
		cmd.Stderr = lockingWriter{w: &stderr, mu: &outputMu}
	}
	if err := cmd.Start(); err != nil {
		result := CommandResult{
			Stderr:     stderr.String(),
			Duration:   time.Since(started),
			ExitCode:   1,
			Executable: name,
		}
		return result, ToolError{Kind: ErrorStart, Tool: req.Name, Operation: operation(req.Name, req.Args), Stderr: result.Stderr, Err: err}
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()

	err = waitForCommand(stdctx, req, cmd, done)
	outputMu.Lock()
	result := CommandResult{
		Stdout:     stdout.String(),
		Stderr:     stderr.String(),
		Duration:   time.Since(started),
		ExitCode:   0,
		Executable: name,
	}
	outputMu.Unlock()
	if err == errCommandCompletedExternally {
		result.CompletedExternally = true
		return result, nil
	}
	if stdctx.Err() == context.DeadlineExceeded {
		result.TimedOut = true
		return result, ToolError{Kind: ErrorTimeout, Tool: req.Name, Operation: operation(req.Name, req.Args), Stderr: result.Stderr, Err: stdctx.Err()}
	}
	if stdctx.Err() == context.Canceled {
		return result, ToolError{Kind: ErrorTimeout, Tool: req.Name, Operation: operation(req.Name, req.Args), Stderr: result.Stderr, Err: stdctx.Err()}
	}
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			result.ExitCode = exitErr.ExitCode()
			return result, ToolError{Kind: ErrorExit, Tool: req.Name, Operation: operation(req.Name, req.Args), ExitCode: result.ExitCode, Stderr: result.Stderr, Err: err}
		}
		result.ExitCode = 1
		return result, ToolError{Kind: ErrorStart, Tool: req.Name, Operation: operation(req.Name, req.Args), Stderr: result.Stderr, Err: err}
	}
	return result, nil
}

var errCommandCompletedExternally = errors.New("command completed externally")

type lockingWriter struct {
	w  ioWriter
	mu *sync.Mutex
}

type ioWriter interface {
	Write([]byte) (int, error)
}

func (w lockingWriter) Write(p []byte) (int, error) {
	if w.mu != nil {
		w.mu.Lock()
		defer w.mu.Unlock()
	}
	return w.w.Write(p)
}

func waitForCommand(ctx context.Context, req Command, cmd *exec.Cmd, done <-chan error) error {
	var ticker *time.Ticker
	var tick <-chan time.Time
	if req.CompletionCheck != nil {
		interval := req.CompletionCheckInterval
		if interval <= 0 {
			interval = 2 * time.Second
		}
		ticker = time.NewTicker(interval)
		defer ticker.Stop()
		tick = ticker.C
	}
	for {
		select {
		case err := <-done:
			return err
		case <-ctx.Done():
			terminateCommand(cmd)
			return waitAfterTermination(done, ctx.Err())
		case <-tick:
			complete, err := req.CompletionCheck()
			if err != nil {
				continue
			}
			if complete {
				terminateCommand(cmd)
				_ = waitAfterTermination(done, errCommandCompletedExternally)
				return errCommandCompletedExternally
			}
		}
	}
}

func waitAfterTermination(done <-chan error, fallback error) error {
	select {
	case err := <-done:
		if fallback == errCommandCompletedExternally {
			return fallback
		}
		return err
	case <-time.After(10 * time.Second):
		return fallback
	}
}

func (r SystemRunner) RunShell(ctx context.Context, dir, command string, timeout time.Duration) (CommandResult, error) {
	if runtime.GOOS == "windows" {
		return r.Run(ctx, Command{Dir: dir, Name: "cmd", Args: []string{"/c", command}, Timeout: timeout})
	}
	return r.Run(ctx, Command{Dir: dir, Name: "sh", Args: []string{"-c", command}, Timeout: timeout})
}

func (r SystemRunner) resolve(name string) (string, error) {
	if filepath.IsAbs(name) || strings.ContainsAny(name, `/\`) {
		return name, nil
	}
	ad := r.Adapter
	if ad == nil {
		ad = osadapter.Detect()
	}
	if resolved, err := ad.FindTool(name); err == nil {
		return resolved, nil
	}
	return exec.LookPath(name)
}

func operation(name string, args []string) string {
	if len(args) == 0 {
		return name
	}
	return strings.TrimSpace(name + " " + strings.Join(args, " "))
}
