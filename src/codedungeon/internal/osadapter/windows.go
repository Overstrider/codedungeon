//go:build windows

package osadapter

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

type windowsAdapter struct{ baseAdapter }

// Detect returns the concrete adapter for this build.
func Detect() Adapter {
	return windowsAdapter{baseAdapter{osName: "windows"}}
}

func (windowsAdapter) PathSep() string       { return ";" }
func (windowsAdapter) ExecutableExt() string { return ".exe" }

// Well-known install locations that may not be on PATH for bash-on-Windows users.
var extraHints = []string{
	`C:\tools\gh`,
	`C:\Program Files\GitHub CLI`,
	`C:\Program Files\GitHub CLI\bin`,
	`C:\Program Files\Git\mingw64\bin`,
	`C:\Program Files\Git\usr\bin`,
}

func (windowsAdapter) FindTool(name string) (string, error) {
	if p, ok := findToolFromEnv(name); ok {
		return p, nil
	}
	if p, err := exec.LookPath(name); err == nil {
		return p, nil
	}
	if filepath.Ext(name) == "" {
		name += ".exe"
	}
	if p, err := exec.LookPath(name); err == nil {
		return p, nil
	}
	for _, h := range windowsToolHints(name) {
		if candidate, ok := resolveToolCandidate(h, name); ok {
			return candidate, nil
		}
	}
	return "", errNotFound{Tool: name}
}

func findToolFromEnv(name string) (string, bool) {
	base := strings.TrimSuffix(filepath.Base(name), filepath.Ext(name))
	base = strings.ToUpper(strings.ReplaceAll(base, "-", "_"))
	for _, key := range []string{"CODEDUNGEON_" + base + "_PATH", base + "_PATH"} {
		if p, ok := resolveToolCandidate(os.Getenv(key), name); ok {
			return p, true
		}
	}
	return "", false
}

func windowsToolHints(name string) []string {
	hints := append([]string{}, extraHints...)
	if strings.EqualFold(strings.TrimSuffix(filepath.Base(name), filepath.Ext(name)), "gh") {
		hints = append(hints,
			filepath.Join(os.Getenv("ProgramFiles"), "GitHub CLI"),
			filepath.Join(os.Getenv("ProgramFiles"), "GitHub CLI", "bin"),
			filepath.Join(os.Getenv("ProgramFiles(x86)"), "GitHub CLI"),
			filepath.Join(os.Getenv("LOCALAPPDATA"), "Programs", "GitHub CLI"),
			filepath.Join(os.Getenv("LOCALAPPDATA"), "Microsoft", "WinGet", "Links"),
			filepath.Join(os.Getenv("LOCALAPPDATA"), "Microsoft", "WindowsApps"),
			filepath.Join(os.Getenv("LOCALAPPDATA"), "Temp", "gh-portable", "bin"),
			filepath.Join(os.Getenv("TEMP"), "gh-portable", "bin"),
			filepath.Join(os.Getenv("TMP"), "gh-portable", "bin"),
			filepath.Join(os.Getenv("USERPROFILE"), "scoop", "shims"),
			filepath.Join(os.Getenv("ProgramData"), "chocolatey", "bin"),
		)
	}
	return hints
}

func resolveToolCandidate(hint, name string) (string, bool) {
	hint = strings.TrimSpace(strings.Trim(hint, `"`))
	if hint == "" {
		return "", false
	}
	if filepath.Ext(name) == "" {
		name += ".exe"
	}
	candidates := []string{hint}
	if info, err := os.Stat(hint); err == nil && info.IsDir() {
		candidates = []string{filepath.Join(hint, name)}
	} else if filepath.Ext(hint) == "" {
		candidates = append(candidates, filepath.Join(hint, name))
	}
	for _, candidate := range candidates {
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			return candidate, true
		}
	}
	return "", false
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
