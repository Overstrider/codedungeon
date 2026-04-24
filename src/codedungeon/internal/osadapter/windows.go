//go:build windows

package osadapter

import (
	"os"
	"os/exec"
	"path/filepath"
)

type windowsAdapter struct{ baseAdapter }

// Detect returns the concrete adapter for this build.
func Detect() Adapter {
	return windowsAdapter{baseAdapter{osName: "windows"}}
}

func (windowsAdapter) PathSep() string      { return ";" }
func (windowsAdapter) ExecutableExt() string { return ".exe" }

// Well-known install locations that may not be on PATH for bash-on-Windows users
// (e.g. gh.exe at /c/tools/gh/gh.exe in loldi's setup).
var extraHints = []string{
	`C:\tools\gh`,
	`C:\Program Files\GitHub CLI`,
	`C:\Program Files\Git\mingw64\bin`,
	`C:\Program Files\Git\usr\bin`,
}

func (windowsAdapter) FindTool(name string) (string, error) {
	if filepath.Ext(name) == "" {
		name += ".exe"
	}
	if p, err := exec.LookPath(name); err == nil {
		return p, nil
	}
	for _, h := range extraHints {
		candidate := filepath.Join(h, name)
		if _, err := os.Stat(candidate); err == nil {
			return candidate, nil
		}
	}
	return "", errNotFound{Tool: name}
}

func (a windowsAdapter) RunShell(dir, cmd string) (string, string, error) {
	return runCmd(dir, "cmd", "/c", cmd)
}

func (a windowsAdapter) RunExec(dir, name string, args ...string) (string, string, error) {
	if filepath.Ext(name) == "" {
		name += ".exe"
	}
	return runCmd(dir, name, args...)
}
