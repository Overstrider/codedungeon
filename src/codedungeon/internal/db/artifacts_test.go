package db

import (
	"path/filepath"
	"testing"
)

func TestInstalledArtifactStoresProviderPackAndInstallPath(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "codedungeon.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if err := s.Init(); err != nil {
		t.Fatal(err)
	}

	in := InstalledArtifact{
		RelPath:       "agents/cd_dev_worker.toml",
		InstallPath:   ".codex/agents/cd_dev_worker.toml",
		SHA256:        "abc",
		BinaryVersion: "test",
		Provider:      "codex",
		PackID:        "codedungeon-codex",
		PackVersion:   "1",
		Kind:          "agent",
		LogicalName:   "cd_dev_worker",
		InstalledAt:   123,
	}
	if err := s.UpsertArtifact(in); err != nil {
		t.Fatal(err)
	}

	got, err := s.GetArtifact("agents/cd_dev_worker.toml")
	if err != nil {
		t.Fatal(err)
	}
	if got == nil {
		t.Fatal("artifact not found")
	}
	if got.Provider != in.Provider || got.PackID != in.PackID || got.InstallPath != in.InstallPath || got.Kind != in.Kind || got.LogicalName != in.LogicalName {
		t.Fatalf("artifact metadata = %+v, want %+v", got, in)
	}
}

func TestMigrateV2DatabaseWithoutArtifactsTableToV6(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "codedungeon.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	if _, err := s.DB.Exec(`
		CREATE TABLE meta (key TEXT PRIMARY KEY, value TEXT NOT NULL);
		INSERT INTO meta(key, value) VALUES ('schema_version', '2');
	`); err != nil {
		t.Fatal(err)
	}

	if err := s.Init(); err != nil {
		t.Fatal(err)
	}
	ver, err := s.SchemaVersion()
	if err != nil {
		t.Fatal(err)
	}
	if ver != "6" {
		t.Fatalf("schema version = %q, want 6", ver)
	}
	for _, key := range []string{"model_reasoning_effort", "model_fast_effort"} {
		if _, err := s.GetMeta(key); err != nil {
			t.Fatalf("missing %s: %v", key, err)
		}
	}
}

func TestMigrateV4RenamesClaudeProviderMetadataToCanonicalName(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "codedungeon.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	if _, err := s.DB.Exec(`
		CREATE TABLE meta (key TEXT PRIMARY KEY, value TEXT NOT NULL);
		INSERT INTO meta(key, value) VALUES ('schema_version', '4');
		CREATE TABLE installed_artifacts (
			rel_path TEXT PRIMARY KEY,
			install_path TEXT NOT NULL DEFAULT '',
			sha256 TEXT NOT NULL,
			binary_version TEXT NOT NULL,
			provider TEXT NOT NULL DEFAULT 'claude-code',
			pack_id TEXT NOT NULL DEFAULT 'codedungeon-claude-code',
			pack_version TEXT NOT NULL DEFAULT '1',
			kind TEXT NOT NULL DEFAULT '',
			logical_name TEXT NOT NULL DEFAULT '',
			user_modified INTEGER NOT NULL DEFAULT 0,
			installed_at INTEGER NOT NULL
		);
		INSERT INTO installed_artifacts(rel_path, install_path, sha256, binary_version, provider, pack_id, pack_version, kind, logical_name, user_modified, installed_at)
		VALUES ('commands/minidungeon.md', '.claude/commands/minidungeon.md', 'abc', 'test', 'claude-code', 'codedungeon-claude-code', '1', 'command', 'minidungeon', 0, 123);
	`); err != nil {
		t.Fatal(err)
	}

	if err := s.Migrate(); err != nil {
		t.Fatal(err)
	}

	got, err := s.GetArtifact("commands/minidungeon.md")
	if err != nil {
		t.Fatal(err)
	}
	if got.Provider != "claude" || got.PackID != "codedungeon-claude" {
		t.Fatalf("artifact metadata = provider %q pack %q, want claude/codedungeon-claude", got.Provider, got.PackID)
	}
}
