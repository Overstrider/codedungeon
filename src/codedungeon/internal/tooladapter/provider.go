package tooladapter

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/loldinis/codedungeon/internal/clauderuntime"
)

type ProviderRunRequest struct {
	Provider          string
	Root              string
	Model             string
	Prompt            string
	Env               []string
	OutputLastMessage string
	Stream            bool
}

type ProviderRunner interface {
	Run(ctx context.Context, req ProviderRunRequest) error
}

type CLIProviderRunner struct {
	Runner CommandRunner
}

func NewProviderRunner(runner CommandRunner) ProviderRunner {
	if runner == nil {
		runner = NewSystemRunner()
	}
	return CLIProviderRunner{Runner: runner}
}

func (r CLIProviderRunner) Run(ctx context.Context, req ProviderRunRequest) error {
	providerName := strings.TrimSpace(req.Provider)
	if providerName == "" {
		return fmt.Errorf("provider is required")
	}
	root := strings.TrimSpace(req.Root)
	if root == "" {
		root = "."
	}
	switch providerName {
	case "codex":
		args := []string{"exec", "--cd", root, "--dangerously-bypass-approvals-and-sandbox", "--enable", "multi_agent_v2"}
		if strings.TrimSpace(req.OutputLastMessage) != "" {
			args = append(args, "--output-last-message", filepath.Clean(req.OutputLastMessage))
		}
		args = append(args, "-")
		return r.run(ctx, Command{Dir: root, Name: "codex", Args: args, Stdin: req.Prompt, Env: req.Env, Stdout: streamOut(req.Stream), Stderr: streamErr(req.Stream)})
	case "claude":
		args := []string{
			"--setting-sources", "project,local",
			"--strict-mcp-config",
			"-p", "Read the CodeDungeon runner prompt from stdin and execute it.",
			"--output-format", "stream-json",
			"--verbose",
			"--dangerously-skip-permissions",
		}
		if model := strings.TrimSpace(req.Model); model != "" {
			args = append(args, "--model", model)
		}
		env := clauderuntime.MergeEnv(req.Env, clauderuntime.ModelEnv(req.Model))
		return r.run(ctx, Command{Dir: root, Name: "claude", Args: args, Stdin: req.Prompt, Env: env, Stdout: streamOut(req.Stream), Stderr: streamErr(req.Stream)})
	default:
		return fmt.Errorf("unsupported provider %q", providerName)
	}
}

func (r CLIProviderRunner) run(ctx context.Context, cmd Command) error {
	var stdout, stderr bytes.Buffer
	var stdoutFilter *codeAgentNotificationFilter
	if cmd.Stdout != nil {
		stdoutFilter = newCodeAgentNotificationFilter(cmd.Stdout)
		cmd.Stdout = io.MultiWriter(stdoutFilter, &stdout)
	}
	if cmd.Stderr != nil {
		cmd.Stderr = io.MultiWriter(cmd.Stderr, &stderr)
	}
	result, err := r.Runner.Run(ctx, cmd)
	if stdoutFilter != nil {
		_ = stdoutFilter.Flush()
	}
	if err != nil {
		details := strings.TrimSpace(stripCodeAgentNotifications(strings.Join(nonEmptyStrings(result.Stderr, stderr.String(), result.Stdout, stdout.String()), "\n")))
		if details != "" && !strings.Contains(err.Error(), details) {
			return fmt.Errorf("%w: %s", err, details)
		}
	}
	return err
}

func nonEmptyStrings(values ...string) []string {
	var out []string
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			out = append(out, strings.TrimSpace(value))
		}
	}
	return out
}

type codeAgentNotificationFilter struct {
	w        io.Writer
	pending  bytes.Buffer
	dropping bool
}

func newCodeAgentNotificationFilter(w io.Writer) *codeAgentNotificationFilter {
	return &codeAgentNotificationFilter{w: w}
}

func (f *codeAgentNotificationFilter) Write(p []byte) (int, error) {
	_, _ = f.pending.Write(p)
	for {
		line, ok := takeLine(&f.pending)
		if !ok {
			break
		}
		if f.shouldDrop(line) {
			continue
		}
		if _, err := f.w.Write([]byte(line)); err != nil {
			return len(p), err
		}
	}
	return len(p), nil
}

func (f *codeAgentNotificationFilter) Flush() error {
	if f.pending.Len() == 0 {
		return nil
	}
	line := f.pending.String()
	f.pending.Reset()
	if f.shouldDrop(line) {
		return nil
	}
	_, err := f.w.Write([]byte(line))
	return err
}

func (f *codeAgentNotificationFilter) shouldDrop(line string) bool {
	if isCodeAgentNotificationLine(line) {
		if strings.Contains(line, "</subagent_notification>") || strings.Contains(line, `</subagent_notification>`) {
			f.dropping = false
		} else {
			f.dropping = true
		}
		return true
	}
	if f.dropping {
		if strings.Contains(line, "</subagent_notification>") || strings.Contains(line, `</subagent_notification>`) {
			f.dropping = false
		}
		return true
	}
	return false
}

func takeLine(buf *bytes.Buffer) (string, bool) {
	data := buf.Bytes()
	for i, b := range data {
		if b == '\n' {
			line := string(data[:i+1])
			rest := append([]byte(nil), data[i+1:]...)
			buf.Reset()
			_, _ = buf.Write(rest)
			return line, true
		}
	}
	return "", false
}

func stripCodeAgentNotifications(text string) string {
	var out strings.Builder
	dropping := false
	for _, line := range strings.SplitAfter(text, "\n") {
		if isCodeAgentNotificationLine(line) {
			dropping = !strings.Contains(line, "</subagent_notification>")
			continue
		}
		if dropping {
			if strings.Contains(line, "</subagent_notification>") {
				dropping = false
			}
			continue
		}
		out.WriteString(line)
	}
	return out.String()
}

func isCodeAgentNotificationLine(line string) bool {
	return strings.Contains(line, "subagent_notification") ||
		(strings.Contains(line, `"agent_path"`) && strings.Contains(line, `"status"`))
}

func streamOut(stream bool) io.Writer {
	if stream {
		return os.Stdout
	}
	return nil
}

func streamErr(stream bool) io.Writer {
	if stream {
		return os.Stderr
	}
	return nil
}
