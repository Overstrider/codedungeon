//go:build !linux && !windows && !darwin

package osadapter

import (
	"os/exec"
	"runtime"
)

type genericAdapter struct{ baseAdapter }

// Detect returns a best-effort POSIX adapter for unsupported OSes.
func Detect() Adapter {
	return genericAdapter{baseAdapter{osName: runtime.GOOS}}
}

func (genericAdapter) PathSep() string      { return ":" }
func (genericAdapter) ExecutableExt() string { return "" }

func (genericAdapter) FindTool(name string) (string, error) {
	p, err := exec.LookPath(name)
	if err != nil {
		return "", errNotFound{Tool: name}
	}
	return p, nil
}

func (a genericAdapter) RunShell(dir, cmd string) (string, string, error) {
	return runCmd(dir, "sh", "-c", cmd)
}

func (a genericAdapter) RunExec(dir, name string, args ...string) (string, string, error) {
	return runCmd(dir, name, args...)
}
