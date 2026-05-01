package taskexec

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

type ShellVerifier struct{}

func (ShellVerifier) Verify(ctx context.Context, req VerifyRequest) ([]VerificationResult, error) {
	var results []VerificationResult
	for _, command := range req.Commands {
		command = strings.TrimSpace(command)
		if command == "" {
			continue
		}
		if err := req.Policy.ValidateShellCommand(command); err != nil {
			return results, err
		}
		result := runVerificationCommand(ctx, req.Root, req.Repo, command)
		results = append(results, result)
		if result.Status != "PASS" {
			return results, fmt.Errorf("verification command failed: %s", command)
		}
	}
	return results, nil
}

func runVerificationCommand(ctx context.Context, root, repo, command string) VerificationResult {
	cwd := root
	if strings.TrimSpace(repo) != "" && repo != "." {
		cwd = filepath.Join(root, repo)
	}
	logDir := filepath.Join(root, ".codedungeon", "execute", "logs")
	_ = os.MkdirAll(logDir, 0o755)
	logPath := filepath.Join(logDir, fmt.Sprintf("verify-%d.log", time.Now().UnixNano()))

	var cmd *exec.Cmd
	if isWindows() {
		cmd = exec.CommandContext(ctx, "powershell", "-NoProfile", "-Command", command)
	} else {
		cmd = exec.CommandContext(ctx, "sh", "-c", command)
	}
	cmd.Dir = cwd
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	status := "PASS"
	if err != nil {
		status = "FAIL"
	}
	body := fmt.Sprintf("$ %s\n\n[stdout]\n%s\n\n[stderr]\n%s\n", command, stdout.String(), stderr.String())
	if err != nil {
		body += fmt.Sprintf("\n[error]\n%v\n", err)
	}
	_ = os.WriteFile(logPath, []byte(body), 0o644)
	result := VerificationResult{Command: command, Status: status, LogPath: logPath}
	if err != nil {
		result.Error = err.Error()
	}
	return result
}

func isWindows() bool {
	return os.PathSeparator == '\\'
}
