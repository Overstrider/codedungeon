//go:build linux

package osadapter

import (
	"os/exec"
)

type linuxAdapter struct{ baseAdapter }

// Detect returns the concrete adapter for this build.
func Detect() Adapter {
	return linuxAdapter{baseAdapter{osName: "linux"}}
}

func (linuxAdapter) PathSep() string      { return ":" }
func (linuxAdapter) ExecutableExt() string { return "" }

func (linuxAdapter) FindTool(name string) (string, error) {
	p, err := exec.LookPath(name)
	if err != nil {
		return "", errNotFound{Tool: name}
	}
	return p, nil
}

func (a linuxAdapter) RunShell(dir, cmd string) (string, string, error) {
	return runCmd(dir, "bash", "-c", cmd)
}

func (a linuxAdapter) RunExec(dir, name string, args ...string) (string, string, error) {
	return runCmd(dir, name, args...)
}
