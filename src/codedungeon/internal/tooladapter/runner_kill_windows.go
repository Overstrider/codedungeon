//go:build windows

package tooladapter

import (
	"os/exec"
	"strconv"
)

func prepareCommandForTreeKill(_ *exec.Cmd) {}

func terminateCommand(cmd *exec.Cmd) {
	if cmd == nil || cmd.Process == nil {
		return
	}
	_ = exec.Command("taskkill", "/T", "/F", "/PID", strconv.Itoa(cmd.Process.Pid)).Run()
	_ = cmd.Process.Kill()
}
