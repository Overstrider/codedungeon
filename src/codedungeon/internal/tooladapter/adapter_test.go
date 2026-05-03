package tooladapter

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type recordingRunner struct {
	calls   []Command
	results []CommandResult
	errs    []error
}

func (r *recordingRunner) Run(_ context.Context, cmd Command) (CommandResult, error) {
	r.calls = append(r.calls, cmd)
	if len(r.errs) > 0 {
		err := r.errs[0]
		r.errs = r.errs[1:]
		if err != nil {
			return CommandResult{}, err
		}
	}
	if len(r.results) == 0 {
		return CommandResult{}, nil
	}
	result := r.results[0]
	r.results = r.results[1:]
	return result, nil
}

type failingStreamingRunner struct {
	stdout string
}

func (r failingStreamingRunner) Run(_ context.Context, cmd Command) (CommandResult, error) {
	if cmd.Stdout != nil {
		_, _ = fmt.Fprint(cmd.Stdout, r.stdout)
	}
	return CommandResult{ExitCode: 1}, ToolError{Kind: ErrorExit, Tool: cmd.Name, Operation: cmd.Name, ExitCode: 1, Err: errors.New("exit status 1")}
}

type successfulStreamingRunner struct {
	stdout string
}

func (r successfulStreamingRunner) Run(_ context.Context, cmd Command) (CommandResult, error) {
	if cmd.Stdout != nil {
		_, _ = fmt.Fprint(cmd.Stdout, r.stdout)
	}
	return CommandResult{}, nil
}

func TestGitClientCurrentBranchUsesTypedRunner(t *testing.T) {
	runner := &recordingRunner{results: []CommandResult{{Stdout: "feature/adapter\n"}}}
	git := NewGitClient(runner)

	branch, err := git.CurrentBranch(context.Background(), "repo")
	if err != nil {
		t.Fatal(err)
	}

	if branch != "feature/adapter" {
		t.Fatalf("branch = %q, want feature/adapter", branch)
	}
	if len(runner.calls) != 1 {
		t.Fatalf("calls = %d, want 1", len(runner.calls))
	}
	call := runner.calls[0]
	if call.Dir != "repo" || call.Name != "git" || strings.Join(call.Args, " ") != "branch --show-current" {
		t.Fatalf("unexpected git call: %+v", call)
	}
}

func TestGitClientAddAllExcludesRuntimeDatabases(t *testing.T) {
	runner := &recordingRunner{}
	git := NewGitClient(runner)

	if err := git.AddAll(context.Background(), "repo"); err != nil {
		t.Fatal(err)
	}

	if len(runner.calls) != 1 {
		t.Fatalf("calls = %d, want 1", len(runner.calls))
	}
	call := runner.calls[0]
	got := strings.Join(call.Args, " ")
	want := "add -A -- . :(exclude).codedungeon/*.db :(exclude).codedungeon/*.db-journal :(exclude).codedungeon/logs/**"
	if call.Dir != "repo" || call.Name != "git" || got != want {
		t.Fatalf("unexpected git add call: %+v", call)
	}
}

func TestGitHubClientAuthStatusReturnsTypedError(t *testing.T) {
	runner := &recordingRunner{errs: []error{ToolError{Kind: ErrorExit, Tool: "gh", Operation: "auth status", ExitCode: 1, Stderr: "not logged in"}}}
	gh := NewGitHubClient(runner)

	err := gh.AuthStatus(context.Background(), "repo")
	if err == nil {
		t.Fatal("expected auth error")
	}
	var toolErr ToolError
	if !AsToolError(err, &toolErr) {
		t.Fatalf("expected ToolError, got %T: %v", err, err)
	}
	if toolErr.Tool != "gh" || toolErr.Operation != "auth status" || toolErr.ExitCode != 1 {
		t.Fatalf("unexpected typed error: %+v", toolErr)
	}
	if len(runner.calls) != 1 {
		t.Fatalf("calls = %d, want 1", len(runner.calls))
	}
	call := runner.calls[0]
	if call.Dir != "repo" || call.Name != "gh" || strings.Join(call.Args, " ") != "auth status" {
		t.Fatalf("unexpected gh call: %+v", call)
	}
}

func TestClaudeProviderRunnerSendsPromptOverStdin(t *testing.T) {
	runner := &recordingRunner{}
	longPrompt := strings.Repeat("CodeDungeon child prompt\n", 200)
	providerRunner := NewProviderRunner(runner)

	if err := providerRunner.Run(context.Background(), ProviderRunRequest{
		Provider: "claude",
		Root:     "repo",
		Model:    "claude-sonnet-4-6",
		Prompt:   longPrompt,
	}); err != nil {
		t.Fatal(err)
	}

	if len(runner.calls) != 1 {
		t.Fatalf("calls = %d, want 1", len(runner.calls))
	}
	call := runner.calls[0]
	if call.Name != "claude" || call.Dir != "repo" {
		t.Fatalf("unexpected claude call: %+v", call)
	}
	if call.Stdin != longPrompt {
		t.Fatalf("claude stdin length = %d, want %d", len(call.Stdin), len(longPrompt))
	}
	if strings.Contains(strings.Join(call.Args, " "), longPrompt) {
		t.Fatalf("claude args contain the full prompt; prompt must travel over stdin: %+v", call.Args)
	}
	if !containsArg(call.Args, "-p") {
		t.Fatalf("claude args should use documented -p query mode, got %+v", call.Args)
	}
	if containsArg(call.Args, "--output-format") && containsArg(call.Args, "stream-json") && !containsArg(call.Args, "--verbose") {
		t.Fatalf("claude stream-json mode requires --verbose, got %+v", call.Args)
	}
	if !containsArg(call.Args, "--model") || !containsArg(call.Args, "claude-sonnet-4-6") {
		t.Fatalf("claude args missing explicit configured model: %+v", call.Args)
	}
	for _, want := range []string{"--setting-sources", "project,local", "--strict-mcp-config"} {
		if !containsArg(call.Args, want) {
			t.Fatalf("claude args missing project-local isolation arg %q: %+v", want, call.Args)
		}
	}
	if containsArg(call.Args, "--fallback-model") {
		t.Fatalf("claude args must not include fallback model: %+v", call.Args)
	}
	for _, want := range []string{
		"CLAUDE_CODE_SUBAGENT_MODEL=claude-sonnet-4-6",
		"ANTHROPIC_MODEL=claude-sonnet-4-6",
		"ANTHROPIC_DEFAULT_SONNET_MODEL=claude-sonnet-4-6",
		"ANTHROPIC_DEFAULT_OPUS_MODEL=claude-sonnet-4-6",
		"ANTHROPIC_DEFAULT_HAIKU_MODEL=claude-sonnet-4-6",
	} {
		if !containsEnv(call.Env, want) {
			t.Fatalf("claude env missing %q: %+v", want, call.Env)
		}
	}
}

func TestClaudeProviderRunnerIncludesStreamStdoutInFailure(t *testing.T) {
	runner := NewProviderRunner(failingStreamingRunner{
		stdout: `{"type":"result","subtype":"error","error":{"type":"rate_limit_error","message":"out_of_credits"}}` + "\n",
	})

	err := runner.Run(context.Background(), ProviderRunRequest{
		Provider: "claude",
		Root:     "repo",
		Model:    "claude-sonnet-4-6",
		Prompt:   "do work",
		Stream:   true,
	})
	if err == nil {
		t.Fatal("expected provider failure")
	}
	if !strings.Contains(err.Error(), "out_of_credits") || !strings.Contains(err.Error(), "rate_limit_error") {
		t.Fatalf("provider error lost stream stdout details: %v", err)
	}
}

func TestClaudeProviderRunnerSuppressesCodeAgentNotificationsFromStream(t *testing.T) {
	stream := strings.Join([]string{
		"visible before",
		`<subagent_notification>`,
		`{"agent_path":"/root/explore_run_report","status":{"completed":"Ready. What task?"}}`,
		`</subagent_notification>`,
		"visible after",
		"",
	}, "\n")
	runner := NewProviderRunner(successfulStreamingRunner{stdout: stream})
	out := captureStdout(t, func() {
		if err := runner.Run(context.Background(), ProviderRunRequest{
			Provider: "claude",
			Root:     "repo",
			Model:    "claude-sonnet-4-6",
			Prompt:   "do work",
			Stream:   true,
		}); err != nil {
			t.Fatal(err)
		}
	})

	if strings.Contains(out, "subagent_notification") || strings.Contains(out, "agent_path") || strings.Contains(out, "Ready. What task?") {
		t.Fatalf("CodeAgent notification leaked to stream stdout:\n%s", out)
	}
	if !strings.Contains(out, "visible before") || !strings.Contains(out, "visible after") {
		t.Fatalf("normal provider stream output was lost:\n%s", out)
	}
}

func TestSystemRunnerCompletesWhenCompletionCheckPasses(t *testing.T) {
	outPath := filepath.Join(t.TempDir(), "artifact.txt")
	started := time.Now()

	result, err := NewSystemRunner().Run(context.Background(), Command{
		Name:                    os.Args[0],
		Args:                    []string{"-test.run=TestSystemRunnerHelperProcess", "--", outPath},
		Env:                     []string{"CODEDUNGEON_HELPER_PROCESS=artifact-sleep"},
		Timeout:                 30 * time.Second,
		CompletionCheckInterval: 10 * time.Millisecond,
		CompletionCheck: func() (bool, error) {
			_, err := os.Stat(outPath)
			return err == nil, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !result.CompletedExternally {
		t.Fatalf("CompletedExternally = false, want true")
	}
	if time.Since(started) > 5*time.Second {
		t.Fatalf("runner waited for sleeping process instead of artifact completion; duration=%s", time.Since(started))
	}
}

func TestSystemRunnerHelperProcess(t *testing.T) {
	if os.Getenv("CODEDUNGEON_HELPER_PROCESS") != "artifact-sleep" {
		return
	}
	args := os.Args
	outPath := args[len(args)-1]
	if err := os.WriteFile(outPath, []byte("ready"), 0o644); err != nil {
		os.Exit(2)
	}
	time.Sleep(time.Minute)
	os.Exit(0)
}

func containsArg(args []string, want string) bool {
	for _, arg := range args {
		if arg == want {
			return true
		}
	}
	return false
}

func containsEnv(env []string, want string) bool {
	for _, item := range env {
		if item == want {
			return true
		}
	}
	return false
}

func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	original := os.Stdout
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = writer
	fn()
	_ = writer.Close()
	os.Stdout = original
	body, err := io.ReadAll(reader)
	_ = reader.Close()
	if err != nil {
		t.Fatal(err)
	}
	return string(body)
}
