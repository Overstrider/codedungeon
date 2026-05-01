package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/loldinis/codedungeon/internal/codereview"
	"github.com/loldinis/codedungeon/internal/reviewpipe"
)

func TestCodeReviewCommandRunsStandaloneFromFileRunner(t *testing.T) {
	root := t.TempDir()
	oldWD, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(oldWD) })
	if err := os.Chdir(root); err != nil {
		t.Fatal(err)
	}

	projectContext := filepath.Join(root, "project-context.md")
	taskContext := filepath.Join(root, "task-context.md")
	writeFile(t, projectContext, strings.Repeat("Project context for standalone review integrity and explicit approvals. ", 2))
	writeFile(t, taskContext, strings.Repeat("Task context for implementing a backend and frontend with reviewable behavior. ", 2))

	inputDir := filepath.Join(root, "review-input")
	writeStandaloneReviewFixture(t, inputDir)
	outDir := filepath.Join(root, "review-out")

	cmd := CodeReviewCmd()
	cmd.SetArgs([]string{
		"--url", "https://github.com/acme/example/pull/1",
		"--project-context", projectContext,
		"--task-context", taskContext,
		"--project-rules-status", "approved",
		"--project-rules-digest", "rules-digest",
		"--out", outDir,
		"--runner", "files",
		"--input-dir", inputDir,
	})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}

	result, err := codereview.ValidateResultDir(outDir)
	if err != nil {
		t.Fatal(err)
	}
	if result.Verdict != codereview.VerdictApproved {
		t.Fatalf("verdict = %s", result.Verdict)
	}
	body, err := os.ReadFile(filepath.Join(outDir, "review.md"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(body), "_None._") {
		t.Fatalf("standalone review rendered empty template:\n%s", string(body))
	}
	if strings.Contains(string(body), "#### "+codereview.DefaultPersonas[0]) {
		t.Fatalf("standalone review rendered verbose persona section:\n%s", string(body))
	}
	summaryBody, err := os.ReadFile(filepath.Join(outDir, "review-summary.json"))
	if err != nil {
		t.Fatal(err)
	}
	var summary codereview.ReviewSummary
	if err := json.Unmarshal(summaryBody, &summary); err != nil {
		t.Fatal(err)
	}
	if summary.Verdict != codereview.VerdictApproved || summary.FullArtifacts.ResultJSONPath == "" {
		t.Fatalf("unexpected summary artifact: %+v", summary)
	}
}

func TestPostStandaloneReviewFallsBackToCommentWhenRequestChangesReviewIsRejected(t *testing.T) {
	oldRun := codeReviewRun
	t.Cleanup(func() { codeReviewRun = oldRun })
	var calls [][]string
	codeReviewRun = func(_ string, args ...string) (string, string, error) {
		calls = append(calls, append([]string{}, args...))
		joined := strings.Join(args, " ")
		switch {
		case strings.Contains(joined, "/repos/acme/example/pulls/123/reviews"):
			return "", "GraphQL: Review Can not request changes on your own pull request", errors.New("review rejected")
		case strings.Contains(joined, "/repos/acme/example/issues/123/comments"):
			return `{"id":456,"html_url":"https://github.com/acme/example/pull/123#issuecomment-456"}`, "", nil
		default:
			t.Fatalf("unexpected gh command: %v", args)
			return "", "", nil
		}
	}

	url, id, err := postStandaloneReview("https://github.com/acme/example/pull/123", "body", codereview.VerdictChangesRequested)
	if err != nil {
		t.Fatal(err)
	}
	if url != "https://github.com/acme/example/pull/123#issuecomment-456" || id != "456" {
		t.Fatalf("unexpected post result url=%q id=%q", url, id)
	}
	if len(calls) != 2 {
		t.Fatalf("calls = %d, want review attempt then comment fallback: %#v", len(calls), calls)
	}
	if !strings.Contains(strings.Join(calls[0], " "), "/pulls/123/reviews") {
		t.Fatalf("first call was not request-changes review: %#v", calls)
	}
	if !strings.Contains(strings.Join(calls[1], " "), "/issues/123/comments") {
		t.Fatalf("second call was not issue comment fallback: %#v", calls)
	}
}

func TestCollectTargetContextAutoCompactsLargeDiff(t *testing.T) {
	oldRun := codeReviewRun
	t.Cleanup(func() { codeReviewRun = oldRun })
	codeReviewRun = func(_ string, args ...string) (string, string, error) {
		joined := strings.Join(args, " ")
		switch {
		case strings.Contains(joined, "pr view"):
			return `{"title":"Large PR","url":"https://github.com/acme/example/pull/123","baseRefOid":"aaaaaaaa","headRefOid":"bbbbbbbb","files":[{"path":"src/app.go","additions":120,"deletions":8}]}`, "", nil
		case strings.Contains(joined, "pr diff"):
			return strings.Repeat("diff --git a/src/app.go b/src/app.go\n+very large change body\n", 200), "", nil
		default:
			t.Fatalf("unexpected gh command: %v", args)
			return "", "", nil
		}
	}

	ctx, info := collectTargetContextWithOptions("https://github.com/acme/example/pull/123", targetContextOptions{
		Mode:     "auto",
		MaxBytes: 256,
	})
	if info.Mode != "compact" {
		t.Fatalf("mode = %q, want compact", info.Mode)
	}
	for _, want := range []string{"Context mode: compact", "Large PR", "src/app.go", "Diff omitted"} {
		if !strings.Contains(ctx, want) {
			t.Fatalf("compact context missing %q:\n%s", want, ctx)
		}
	}
	if strings.Contains(ctx, "very large change body") {
		t.Fatalf("compact context leaked full diff:\n%s", ctx)
	}
}

func writeStandaloneReviewResultFixture(t *testing.T, dir string) codereview.Result {
	t.Helper()
	inputDir := filepath.Join(t.TempDir(), "input")
	writeStandaloneReviewFixture(t, inputDir)
	result, err := codereview.Execute(context.Background(), codereview.Request{
		URL:            "https://github.com/acme/example/pull/123",
		ProjectContext: strings.Repeat("Project context for valid standalone review evidence and non-empty approvals. ", 2),
		TaskContext:    strings.Repeat("Task context for valid standalone review evidence and final adjudication. ", 2),
		OutputDir:      dir,
		PRNumber:       "123",
		BaseSHA:        "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		HeadSHA:        "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
		ProjectRules:   codereview.ProjectRulesEnvelope{Status: "approved", Digest: "rules-digest", Read: "yes"},
	}, codereview.FilesRunner{InputDir: inputDir})
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func writeStandaloneReviewFixture(t *testing.T, dir string) {
	t.Helper()
	rules := codereview.ProjectRulesEnvelope{Status: "approved", Digest: "rules-digest", Read: "yes"}
	for _, persona := range codereview.DefaultPersonas {
		writeCodeReviewJSON(t, filepath.Join(dir, "personas", persona+".json"), codereview.PersonaReview{
			Persona:             persona,
			Verdict:             codereview.VerdictApproved,
			Model:               "test-model",
			Provider:            "test",
			SessionID:           "session-" + persona,
			ReviewedFiles:       2,
			ReviewedScope:       []string{"backend/src/main.rs", "frontend/src/app/page.tsx"},
			ApprovalRationale:   persona + " performed a concrete standalone review over the project context, task context, target diff, and verification risks without finding a blocking defect.",
			RisksConsidered:     []string{"correctness reviewed", "verification reviewed"},
			VerificationChecked: []string{"go test ./..."},
			ProjectRules:        rules,
			Findings:            []reviewpipe.Finding{},
		})
	}
	verdicts := map[string]string{}
	for _, persona := range codereview.DefaultPersonas {
		verdicts[persona] = codereview.VerdictApproved
	}
	writeCodeReviewJSON(t, filepath.Join(dir, "review-decision.json"), codereview.Decision{
		Verdict:           codereview.VerdictApproved,
		DecidedBy:         "adjudicator",
		Model:             "test-model",
		Provider:          "test",
		ApprovalRationale: strings.Repeat("The final adjudicator reviewed every persona output, project context, task context, diff scope, and verification evidence before approving. ", 2),
		PersonaVerdicts:   verdicts,
	})
}

func writeCodeReviewJSON(t *testing.T, path string, payload any) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	body, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, body, 0o644); err != nil {
		t.Fatal(err)
	}
}
