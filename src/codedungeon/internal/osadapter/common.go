package osadapter

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// baseAdapter holds shared helpers used by every OS-specific impl.
type baseAdapter struct {
	osName string
}

func (b baseAdapter) OS() string       { return b.osName }
func (b baseAdapter) HomeDir() string  { h, _ := os.UserHomeDir(); return h }
func (b baseAdapter) TempDir() string  { return os.TempDir() }

// runCmd is the cross-OS helper actually doing exec.Command.
// OS-specific adapters wrap this with shell/exec conventions.
func runCmd(dir, name string, args ...string) (string, string, error) {
	c := exec.Command(name, args...)
	if dir != "" {
		c.Dir = dir
	}
	var out, errb bytes.Buffer
	c.Stdout = &out
	c.Stderr = &errb
	err := c.Run()
	return strings.TrimSpace(out.String()), strings.TrimSpace(errb.String()), err
}

// errNotFound is returned when a tool cannot be resolved.
type errNotFound struct{ Tool string }

func (e errNotFound) Error() string { return fmt.Sprintf("tool not found on PATH: %s", e.Tool) }
