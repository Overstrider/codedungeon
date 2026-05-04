package codereview

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/loldinis/codedungeon/internal/tooladapter"
)

type reviewCommandRecorder struct {
	calls []tooladapter.Command
}

func (r *reviewCommandRecorder) Run(_ context.Context, cmd tooladapter.Command) (tooladapter.CommandResult, error) {
	r.calls = append(r.calls, cmd)
	return tooladapter.CommandResult{}, nil
}

type failingClaudeRecorder struct {
	stdout string
	stderr string
}

func (r failingClaudeRecorder) Run(_ context.Context, cmd tooladapter.Command) (tooladapter.CommandResult, error) {
	if cmd.Stdout != nil && r.stdout != "" {
		_, _ = fmt.Fprint(cmd.Stdout, r.stdout)
	}
	if cmd.Stderr != nil && r.stderr != "" {
		_, _ = fmt.Fprint(cmd.Stderr, r.stderr)
	}
	return tooladapter.CommandResult{Stdout: r.stdout, Stderr: r.stderr, ExitCode: 1}, tooladapter.ToolError{Kind: tooladapter.ErrorExit, Tool: "claude", Operation: "claude -p", ExitCode: 1, Stderr: r.stderr, Err: errors.New("exit status 1")}
}

func TestClaudeRunnerSendsReviewPromptOverStdin(t *testing.T) {
	recorder := &reviewCommandRecorder{}
	req := Request{
		URL:            "https://github.com/acme/example/pull/1",
		ProjectContext: strings.Repeat("project context ", 20),
		TaskContext:    strings.Repeat("task context ", 20),
		TargetContext:  "diff --git a/app.go b/app.go",
		ProjectRules:   ProjectRulesEnvelope{Status: "approved", Digest: "rules-digest", Read: "yes"},
	}

	err := ClaudeRunner{WorkDir: "repo", Model: "claude-sonnet-4-6", Runner: recorder}.RunPersona(context.Background(), req, "security", "out.json")
	if err != nil {
		t.Fatal(err)
	}

	if len(recorder.calls) != 1 {
		t.Fatalf("calls = %d, want 1", len(recorder.calls))
	}
	call := recorder.calls[0]
	if call.Name != "claude" || call.Dir != "repo" {
		t.Fatalf("unexpected command: %+v", call)
	}
	if !containsReviewArg(call.Args, "-p") || !containsReviewArg(call.Args, "--output-format") || !containsReviewArg(call.Args, "stream-json") || !containsReviewArg(call.Args, "--verbose") {
		t.Fatalf("claude args missing documented stdin/stream-json flags: %+v", call.Args)
	}
	for _, want := range []string{"--setting-sources", "project,local", "--strict-mcp-config", "--no-session-persistence"} {
		if !containsReviewArg(call.Args, want) {
			t.Fatalf("claude args missing isolation arg %q: %+v", want, call.Args)
		}
	}
	if containsReviewArg(call.Args, "--bare") {
		t.Fatalf("claude args must not use --bare because it disables local auth on this target: %+v", call.Args)
	}
	if !containsReviewArg(call.Args, "--model") || !containsReviewArg(call.Args, "claude-sonnet-4-6") {
		t.Fatalf("claude args missing explicit configured model: %+v", call.Args)
	}
	if containsReviewArg(call.Args, "--fallback-model") {
		t.Fatalf("claude args must not include fallback model: %+v", call.Args)
	}
	for _, want := range []string{
		"CLAUDE_CODE_SUBAGENT_MODEL=claude-sonnet-4-6",
		"ANTHROPIC_MODEL=claude-sonnet-4-6",
		"ANTHROPIC_DEFAULT_SONNET_MODEL=claude-sonnet-4-6",
		"ANTHROPIC_DEFAULT_OPUS_MODEL=claude-sonnet-4-6",
		"ANTHROPIC_DEFAULT_HAIKU_MODEL=claude-sonnet-4-6",
	} {
		if !containsReviewEnv(call.Env, want) {
			t.Fatalf("claude env missing %q: %+v", want, call.Env)
		}
	}
	if call.Timeout <= 0 {
		t.Fatalf("claude command must have a timeout")
	}
	if call.CompletionCheck == nil {
		t.Fatalf("claude command must have an artifact completion check")
	}
	if !strings.Contains(call.Stdin, "Role/persona: security") {
		t.Fatalf("stdin does not contain persona prompt:\n%s", call.Stdin)
	}
	if !strings.Contains(call.Stdin, `"provider": "claude"`) {
		t.Fatalf("stdin should ask Claude reviews to report provider=claude:\n%s", call.Stdin)
	}
	if strings.Contains(strings.Join(call.Args, " "), call.Stdin) {
		t.Fatalf("claude args contain full prompt; review prompt must travel over stdin")
	}
}

func TestClaudeRunnerCompletionCheckAcceptsValidPersonaArtifact(t *testing.T) {
	recorder := &reviewCommandRecorder{}
	rules := ProjectRulesEnvelope{Status: "approved", Digest: "rules-digest", Read: "yes"}
	req := Request{
		URL:            "https://github.com/acme/example/pull/1",
		ProjectContext: strings.Repeat("project context ", 20),
		TaskContext:    strings.Repeat("task context ", 20),
		ProjectRules:   rules,
	}
	outPath := filepath.Join(t.TempDir(), "security.json")

	err := ClaudeRunner{
		WorkDir:                 "repo",
		Runner:                  recorder,
		Timeout:                 time.Minute,
		CompletionCheckInterval: time.Millisecond,
	}.RunPersona(context.Background(), req, "security", outPath)
	if err != nil {
		t.Fatal(err)
	}
	if len(recorder.calls) != 1 {
		t.Fatalf("calls = %d, want 1", len(recorder.calls))
	}

	time.Sleep(10 * time.Millisecond)
	writeReviewArtifact(t, outPath, PersonaReview{
		Persona:             "security",
		Verdict:             VerdictApproved,
		Model:               "claude-test",
		Provider:            "claude",
		SessionID:           "session",
		ReviewedFiles:       1,
		ReviewedScope:       []string{"src/app.go"},
		ApprovalRationale:   strings.Repeat("security reviewed concrete changed files and found no blocking issue after checking auth, persistence, and error handling. ", 2),
		RisksConsidered:     []string{"authorization regression", "data integrity regression"},
		VerificationChecked: []string{"go test ./..."},
		ProjectRules:        rules,
		Findings:            nil,
	})
	ok, err := recorder.calls[0].CompletionCheck()
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("completion check did not accept valid persona artifact")
	}
}

func TestClaudeRunnerClassifiesRateLimitFromStreamJSONStdout(t *testing.T) {
	stream := strings.Join([]string{
		`{"type":"system","subtype":"init"}`,
		`{"type":"result","subtype":"error","error":{"type":"rate_limit_error","message":"rate_limit_error: out_of_credits reset at 2026-05-03T12:00:00Z"}}`,
		"",
	}, "\n")
	err := ClaudeRunner{
		WorkDir: "repo",
		Model:   "claude-sonnet-4-6",
		Runner:  failingClaudeRecorder{stdout: stream},
		Timeout: time.Second,
	}.RunPersona(context.Background(), Request{
		URL:            "https://github.com/acme/example/pull/1",
		ProjectContext: strings.Repeat("project context ", 20),
		TaskContext:    strings.Repeat("task context ", 20),
		ProjectRules:   ProjectRulesEnvelope{Status: "approved", Digest: "rules-digest", Read: "yes"},
	}, "tests", filepath.Join(t.TempDir(), "tests.json"))

	if err == nil {
		t.Fatal("expected Claude rate-limit error")
	}
	var providerErr ProviderFailureError
	if !errors.As(err, &providerErr) {
		t.Fatalf("expected ProviderFailureError, got %T: %v", err, err)
	}
	if providerErr.Kind != FailureProviderRateLimit {
		t.Fatalf("failure kind = %q, want %q", providerErr.Kind, FailureProviderRateLimit)
	}
	if !strings.Contains(providerErr.Message, "out_of_credits") {
		t.Fatalf("provider message lost stdout details: %+v", providerErr)
	}
	if !strings.Contains(err.Error(), "provider_rate_limit") || !strings.Contains(err.Error(), "claude-sonnet-4-6") {
		t.Fatalf("error should include kind and model, got: %v", err)
	}
}

func TestCodexRunnerPromptsReportCodexProvider(t *testing.T) {
	recorder := &reviewCommandRecorder{}
	req := Request{
		URL:            "https://github.com/acme/example/pull/1",
		ProjectContext: strings.Repeat("project context ", 20),
		TaskContext:    strings.Repeat("task context ", 20),
		ProjectRules:   ProjectRulesEnvelope{Status: "approved", Digest: "rules-digest", Read: "yes"},
	}

	err := CodexRunner{WorkDir: "repo", Runner: recorder}.RunPersona(context.Background(), req, "tests", "out.json")
	if err != nil {
		t.Fatal(err)
	}

	if len(recorder.calls) != 1 {
		t.Fatalf("calls = %d, want 1", len(recorder.calls))
	}
	if !strings.Contains(recorder.calls[0].Stdin, `"provider": "codex"`) {
		t.Fatalf("stdin should ask Codex reviews to report provider=codex:\n%s", recorder.calls[0].Stdin)
	}
}

func TestReviewPromptForbidsFindingsOnApprovedVerdict(t *testing.T) {
	prompt := personaPrompt(Request{
		URL:            "https://github.com/acme/example/pull/1",
		ProjectContext: "project context",
		TaskContext:    "task context",
		TargetContext:  "target context",
		ProjectRules:   ProjectRulesEnvelope{Status: "approved", Digest: "rules-digest", Read: "yes"},
	}, "security", "out.json", "claude")

	if !strings.Contains(prompt, `APPROVED requires "findings": [] exactly`) {
		t.Fatalf("prompt must make the APPROVED findings invariant explicit:\n%s", prompt)
	}
	if !strings.Contains(prompt, "Do not put non-blocking observations") {
		t.Fatalf("prompt must tell reviewers where non-blocking observations belong:\n%s", prompt)
	}
}

func containsReviewArg(args []string, want string) bool {
	for _, arg := range args {
		if arg == want {
			return true
		}
	}
	return false
}

func containsReviewEnv(env []string, want string) bool {
	for _, item := range env {
		if item == want {
			return true
		}
	}
	return false
}

func writeReviewArtifact(t *testing.T, path string, payload any) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	body, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	body = append(body, '\n')
	if err := os.WriteFile(path, body, 0o644); err != nil {
		t.Fatal(err)
	}
}
