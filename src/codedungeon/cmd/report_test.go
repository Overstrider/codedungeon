package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/loldinis/codedungeon/internal/db"
)

func TestWriteReportMemoryFilesStoresRunAndPRSummaries(t *testing.T) {
	root := t.TempDir()
	run := &db.Run{ID: 42, Feature: "ship memory", Branch: "feat/memory"}
	repos := []reportRepo{{Name: "api", PRNumber: "123", Verdict: "APPROVED"}}

	if err := writeReportMemoryFiles(root, run, repos, "full report body"); err != nil {
		t.Fatal(err)
	}

	runBody, err := os.ReadFile(filepath.Join(root, ".codedungeon", "memory", "runs", "run-42.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(runBody), "ship memory") || !strings.Contains(string(runBody), "full report body") {
		t.Fatalf("unexpected run memory:\n%s", runBody)
	}

	prBody, err := os.ReadFile(filepath.Join(root, ".codedungeon", "memory", "prs", "pr-123.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(prBody), "PR #123") || !strings.Contains(string(prBody), "api") {
		t.Fatalf("unexpected PR memory:\n%s", prBody)
	}
}

func TestValidateRenderedReportQualityRejectsShallowReport(t *testing.T) {
	err := validateRenderedReportQuality("# Bootstrap Report\n\n## Scope\n")
	if err == nil {
		t.Fatal("shallow report accepted")
	}
	if !strings.Contains(err.Error(), "CodeDungeon PR Report") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateRenderedReportQualityAcceptsPRReport(t *testing.T) {
	report := strings.Join([]string{
		"+------------------------------------------------+",
		"| CodeDungeon PR Report                          |",
		"+------------------------------------------------+",
		"| Status        APPROVED",
		"| Workflow      main-quest",
		"| PR            #123 https://github.com/acme/demo/pull/123",
		"| Branch        feat/demo",
		"| Review        APPROVED",
		"| Cycles        1/9 | last mode: full",
		"+------------------------------------------------+",
		"",
		"Work Done",
		"- Verification: go test ./...: PASS",
		"PROJECT_RULES_STATUS: approved",
		"PROJECT_RULES_DIGEST: abc123",
		"PROJECT_RULES_READ: yes",
	}, "\n")
	if err := validateRenderedReportQuality(report); err != nil {
		t.Fatalf("valid PR report rejected: %v", err)
	}
}
