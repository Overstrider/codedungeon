package artifacts

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"

	"github.com/loldinis/codedungeon/internal/db"
)

func TestRegistryRegistersFileAndDetectsDrift(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, ".codedungeon", "qa", "sessions", "qa-1", "summary.md")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("summary\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	store, err := db.Open(filepath.Join(root, ".codedungeon", "codedungeon.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if err := store.Init(); err != nil {
		t.Fatal(err)
	}
	runID, err := store.CreateRun(&db.Run{Feature: "artifact registry", Branch: "main", Mode: "FULL", ProjectMode: "SINGLE"})
	if err != nil {
		t.Fatal(err)
	}

	registry := NewRegistry(store, root)
	registered, err := registry.Register(Record{
		RunID:     runID,
		Module:    "qa",
		OwnerType: "qa_session",
		OwnerID:   "qa-1",
		Phase:     "6",
		Role:      "summary",
		Kind:      "markdown",
		Path:      path,
		Metadata:  map[string]any{"status": "PASS"},
	})
	if err != nil {
		t.Fatal(err)
	}
	wantSum := sha256.Sum256([]byte("summary\n"))
	if registered.Path != ".codedungeon/qa/sessions/qa-1/summary.md" {
		t.Fatalf("registered path = %q", registered.Path)
	}
	if registered.SHA256 != hex.EncodeToString(wantSum[:]) || registered.Bytes != 8 || registered.MediaType != "text/markdown" {
		t.Fatalf("registered metadata = %+v", registered)
	}

	verifications, err := registry.VerifyRun(runID)
	if err != nil {
		t.Fatal(err)
	}
	if len(verifications) != 1 || verifications[0].Status != VerifyOK {
		t.Fatalf("verify unchanged = %+v", verifications)
	}
	if err := os.WriteFile(path, []byte("changed\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	verifications, err = registry.VerifyRun(runID)
	if err != nil {
		t.Fatal(err)
	}
	if len(verifications) != 1 || verifications[0].Status != VerifyDrifted {
		t.Fatalf("verify changed = %+v", verifications)
	}
}

func TestRegistryRegistersDirectoryWithoutHash(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, ".codedungeon", "reviews", "adv-review")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	store, err := db.Open(filepath.Join(root, ".codedungeon", "codedungeon.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if err := store.Init(); err != nil {
		t.Fatal(err)
	}

	registry := NewRegistry(store, root)
	registered, err := registry.Register(Record{
		Module:    "review",
		OwnerType: "review_evidence",
		OwnerID:   "1",
		Role:      "directory",
		Kind:      "directory",
		Path:      dir,
	})
	if err != nil {
		t.Fatal(err)
	}
	if registered.ArtifactType != "directory" || registered.SHA256 != "" || registered.Bytes != 0 {
		t.Fatalf("directory artifact = %+v", registered)
	}
}
