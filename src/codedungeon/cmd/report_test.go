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
