package taskexec

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/loldinis/codedungeon/internal/tooladapter"
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

	runner := tooladapter.NewSystemRunner()
	var cmdResult tooladapter.CommandResult
	var err error
	if runtime.GOOS == "windows" {
		cmdResult, err = runner.Run(ctx, tooladapter.Command{Dir: cwd, Name: "powershell", Args: []string{"-NoProfile", "-Command", command}})
	} else {
		cmdResult, err = runner.RunShell(ctx, cwd, command, 0)
	}
	status := "PASS"
	if err != nil {
		status = "FAIL"
	}
	body := fmt.Sprintf("$ %s\n\n[stdout]\n%s\n\n[stderr]\n%s\n", command, cmdResult.Stdout, cmdResult.Stderr)
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
