package db

import (
	"path/filepath"
	"testing"
)

func TestStoreRegistersRuntimeArtifactRowsIdempotently(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "codedungeon.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if err := s.Init(); err != nil {
		t.Fatal(err)
	}
	runID, err := s.CreateRun(&Run{Feature: "artifact registry", Branch: "main", Mode: "FULL", ProjectMode: "SINGLE"})
	if err != nil {
		t.Fatal(err)
	}

	input := Artifact{
		RunID:        runID,
		Module:       "qa",
		OwnerType:    "qa_session",
		OwnerID:      "qa-1",
		Phase:        "6",
		Role:         "summary",
		Kind:         "markdown",
		Path:         ".codedungeon/qa/sessions/qa-1/summary.md",
		AbsPath:      "E:/repo/.codedungeon/qa/sessions/qa-1/summary.md",
		ArtifactType: "file",
		MediaType:    "text/markdown",
		SHA256:       "abc",
		Bytes:        10,
		MetadataJSON: `{"status":"PASS"}`,
	}
	if _, err := s.RegisterArtifact(input); err != nil {
		t.Fatal(err)
	}
	input.SHA256 = "def"
	input.Bytes = 20
	if _, err := s.RegisterArtifact(input); err != nil {
		t.Fatal(err)
	}

	byRun, err := s.ArtifactsByRun(runID)
	if err != nil {
		t.Fatal(err)
	}
	if len(byRun) != 1 {
		t.Fatalf("artifacts by run = %d, want 1: %+v", len(byRun), byRun)
	}
	got := byRun[0]
	if got.Module != "qa" || got.OwnerID != "qa-1" || got.SHA256 != "def" || got.Bytes != 20 {
		t.Fatalf("artifact row not updated idempotently: %+v", got)
	}
	if got.MetadataJSON != `{"status":"PASS"}` {
		t.Fatalf("metadata_json = %q", got.MetadataJSON)
	}

	byOwner, err := s.ArtifactsByOwner("qa", "qa_session", "qa-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(byOwner) != 1 || byOwner[0].Path != input.Path {
		t.Fatalf("artifacts by owner = %+v", byOwner)
	}
}

func TestMigrateV15DatabaseCreatesRuntimeArtifactRegistry(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "codedungeon.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if _, err := s.DB.Exec(`
		CREATE TABLE meta (key TEXT PRIMARY KEY, value TEXT NOT NULL);
		INSERT INTO meta(key, value) VALUES ('schema_version', '15');
	`); err != nil {
		t.Fatal(err)
	}

	if err := s.Migrate(); err != nil {
		t.Fatal(err)
	}
	ver, err := s.SchemaVersion()
	if err != nil {
		t.Fatal(err)
	}
	if ver != SchemaVersion {
		t.Fatalf("schema version = %q, want %s", ver, SchemaVersion)
	}
	cols, err := tableColumns(s.DB, "artifacts")
	if err != nil {
		t.Fatal(err)
	}
	for _, col := range []string{"run_id", "module", "owner_type", "owner_id", "role", "kind", "path", "sha256", "bytes", "metadata_json"} {
		if !cols[col] {
			t.Fatalf("artifacts missing column %s", col)
		}
	}
}
