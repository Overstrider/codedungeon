//go:build darwin

package osadapter

import (
	"os/exec"
)

type darwinAdapter struct{ baseAdapter }

// Detect returns the concrete adapter for this build.
func Detect() Adapter {
	return darwinAdapter{baseAdapter{osName: "darwin"}}
}

func (darwinAdapter) PathSep() string      { return ":" }
func (darwinAdapter) ExecutableExt() string { return "" }

func (darwinAdapter) FindTool(name string) (string, error) {
	p, err := exec.LookPath(name)
	if err != nil {
		return "", errNotFound{Tool: name}
	}
	return p, nil
}

func (a darwinAdapter) RunShell(dir, cmd string) (string, string, error) {
	return runCmd(dir, "/bin/bash", "-c", cmd)
}

func (a darwinAdapter) RunExec(dir, name string, args ...string) (string, string, error) {
	return runCmd(dir, name, args...)
}
