package projectcontext

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/loldinis/codedungeon/internal/db"
)

func TestProjectContextProposalApprovalAndAuditUseSQLiteLedger(t *testing.T) {
	root := t.TempDir()
	writeProjectFile(t, filepath.Join(root, "README.md"), "# Demo\n\nExisting project.\n")
	store := openProjectContextStore(t, root)
	runID, err := store.CreateRun(&db.Run{
		Feature:     "Add chat history",
		Branch:      "feat/chat-history",
		Mode:        "FULL",
		ProjectMode: "SINGLE",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.InsertReviewEvidence(db.ReviewEvidence{
		RunID:            runID,
		ReviewDir:        ".codedungeon/code-review",
		ReviewJSONPath:   ".codedungeon/code-review/review.json",
		ManifestPath:     ".codedungeon/code-review/review-request.json",
		Verdict:          "APPROVED",
		PRNumber:         "12",
		BaseSHA:          "base",
		HeadSHA:          "head",
		PersonasExpected: []string{"spec"},
		PersonasRun:      []string{"spec"},
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.InsertVerificationRecord(db.VerificationRecord{
		RunID:   runID,
		Phase:   "6",
		Command: "go test ./...",
		Status:  "PASS",
		LogPath: ".codedungeon/logs/go-test.log",
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.InsertReportEvidence(db.ReportEvidence{
		RunID:      runID,
		ReportPath: ".codedungeon/reports/run-1.md",
		SHA256:     "report-sha",
	}); err != nil {
		t.Fatal(err)
	}

	proposal, err := ProposeFromRun(root, NewSQLiteStore(store), runID)
	if err != nil {
		t.Fatal(err)
	}
	if proposal.Status != StatusPending || proposal.ID == 0 {
		t.Fatalf("proposal = %+v, want pending with id", proposal)
	}
	if !strings.Contains(proposal.ProposedBody, "- Project now has Add chat history.") {
		t.Fatalf("proposal body missing feature bullet:\n%s", proposal.ProposedBody)
	}
	if strings.Contains(proposal.ProposedBody, "review.json") || strings.Contains(proposal.ProposedBody, "go-test.log") {
		t.Fatalf("direct context leaked audit-only details:\n%s", proposal.ProposedBody)
	}
	if _, err := os.Stat(filepath.Join(root, ".codedungeon", "project-context.md")); !os.IsNotExist(err) {
		t.Fatalf("proposal should not write approved context, stat err=%v", err)
	}
	if _, err := os.Stat(filepath.Join(root, ".codedungeon", "project-context.proposal.md")); err != nil {
		t.Fatalf("proposal markdown not written: %v", err)
	}
	audits, err := NewSQLiteStore(store).AuditRecords("review", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(audits) == 0 || !strings.Contains(audits[0].FullJSON, "review.json") {
		t.Fatalf("review audit detail not persisted: %+v", audits)
	}

	version, err := ApproveProposal(root, NewSQLiteStore(store), proposal.ID, "tester")
	if err != nil {
		t.Fatal(err)
	}
	if version.Version != 1 || version.SHA256 == "" {
		t.Fatalf("version = %+v, want v1 with digest", version)
	}
	body, err := os.ReadFile(filepath.Join(root, ".codedungeon", "project-context.md"))
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != proposal.ProposedBody {
		t.Fatalf("approved context body mismatch:\n%s\n--- want ---\n%s", string(body), proposal.ProposedBody)
	}
	status, err := Status(root, NewSQLiteStore(store))
	if err != nil {
		t.Fatal(err)
	}
	if status.Status != StatusApproved || status.ActiveVersion != 1 || status.PendingProposals != 0 {
		t.Fatalf("status after approve = %+v", status)
	}
	writeProjectFile(t, filepath.Join(root, "cmd", "server.go"), "package main\n")
	status, err = Status(root, NewSQLiteStore(store))
	if err != nil {
		t.Fatal(err)
	}
	if status.Status != StatusStale || status.StaleReason != "source digest changed" {
		t.Fatalf("status after source change = %+v, want stale source digest", status)
	}
}

func TestInitExistingAndEmptyProjectGeneratePendingContextProposals(t *testing.T) {
	for _, tc := range []struct {
		name     string
		files    map[string]string
		wantMode string
	}{
		{
			name: "existing",
			files: map[string]string{
				"README.md":      "# Existing\n",
				"backend/go.mod": "module example.com/app\n",
			},
			wantMode: ModeExisting,
		},
		{
			name:     "empty",
			files:    map[string]string{},
			wantMode: ModeEmpty,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			store := openProjectContextStore(t, root)
			for rel, body := range tc.files {
				writeProjectFile(t, filepath.Join(root, rel), body)
			}
			result, err := Init(root, NewSQLiteStore(store), InitOptions{
				Mode:        ModeAuto,
				FirstPrompt: "Build a ChatGPT-style app",
			})
			if err != nil {
				t.Fatal(err)
			}
			if result.Mode != tc.wantMode {
				t.Fatalf("mode = %s, want %s", result.Mode, tc.wantMode)
			}
			if result.Proposal.ID == 0 || result.Proposal.Status != StatusPending {
				t.Fatalf("proposal not pending: %+v", result.Proposal)
			}
			audits, err := NewSQLiteStore(store).AuditRecords("init", 5)
			if err != nil {
				t.Fatal(err)
			}
			if len(audits) == 0 || audits[0].Kind != "init" {
				t.Fatalf("init audit query did not use ledger/FTS: %+v", audits)
			}
			if result.Mode == ModeExisting && len(result.Scan.Files) == 0 {
				t.Fatalf("existing project should include scan summary: %+v", result.Scan)
			}
			if _, err := os.Stat(filepath.Join(root, ".codedungeon", "project-context.proposal.md")); err != nil {
				t.Fatalf("proposal markdown missing: %v", err)
			}
		})
	}
}

func openProjectContextStore(t *testing.T, root string) *db.Store {
	t.Helper()
	store, err := db.Open(filepath.Join(root, ".codedungeon", "codedungeon.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	if err := store.Init(); err != nil {
		t.Fatal(err)
	}
	return store
}

func writeProjectFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}
