package tooladapter

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type Command struct {
	Dir                     string
	Name                    string
	Args                    []string
	Stdin                   string
	Env                     []string
	Timeout                 time.Duration
	Stdout                  io.Writer
	Stderr                  io.Writer
	CompletionCheck         func() (bool, error)
	CompletionCheckInterval time.Duration
}

type CommandResult struct {
	Stdout              string
	Stderr              string
	ExitCode            int
	Duration            time.Duration
	TimedOut            bool
	CompletedExternally bool
	Executable          string
}

type CommandRunner interface {
	Run(ctx context.Context, cmd Command) (CommandResult, error)
}

type ToolErrorKind string

const (
	ErrorInvalid  ToolErrorKind = "invalid"
	ErrorNotFound ToolErrorKind = "not_found"
	ErrorExit     ToolErrorKind = "exit"
	ErrorTimeout  ToolErrorKind = "timeout"
	ErrorStart    ToolErrorKind = "start"
)

type ToolError struct {
	Kind      ToolErrorKind
	Tool      string
	Operation string
	ExitCode  int
	Stderr    string
	Err       error
}

func (e ToolError) Error() string {
	op := strings.TrimSpace(e.Operation)
	if op == "" {
		op = strings.TrimSpace(e.Tool)
	}
	switch e.Kind {
	case ErrorNotFound:
		return fmt.Sprintf("%s not found", op)
	case ErrorTimeout:
		return fmt.Sprintf("%s timed out", op)
	case ErrorExit:
		if strings.TrimSpace(e.Stderr) != "" {
			return fmt.Sprintf("%s failed with exit code %d: %s", op, e.ExitCode, strings.TrimSpace(e.Stderr))
		}
		return fmt.Sprintf("%s failed with exit code %d", op, e.ExitCode)
	default:
		if e.Err != nil {
			return fmt.Sprintf("%s failed: %v", op, e.Err)
		}
		return fmt.Sprintf("%s failed", op)
	}
}

func (e ToolError) Unwrap() error { return e.Err }

func AsToolError(err error, target *ToolError) bool {
	return errors.As(err, target)
}

type FileSystem interface {
	Stat(name string) (os.FileInfo, error)
	ReadFile(name string) ([]byte, error)
	WriteFile(name string, data []byte, perm os.FileMode) error
	MkdirAll(path string, perm os.FileMode) error
	Glob(pattern string) ([]string, error)
	ReadDir(name string) ([]os.DirEntry, error)
}

type OSFileSystem struct{}

func (OSFileSystem) Stat(name string) (os.FileInfo, error) { return os.Stat(name) }
func (OSFileSystem) ReadFile(name string) ([]byte, error)  { return os.ReadFile(name) }
func (OSFileSystem) WriteFile(name string, data []byte, perm os.FileMode) error {
	return os.WriteFile(name, data, perm)
}
func (OSFileSystem) MkdirAll(path string, perm os.FileMode) error { return os.MkdirAll(path, perm) }
func (OSFileSystem) Glob(pattern string) ([]string, error)        { return filepath.Glob(pattern) }
func (OSFileSystem) ReadDir(name string) ([]os.DirEntry, error)   { return os.ReadDir(name) }
