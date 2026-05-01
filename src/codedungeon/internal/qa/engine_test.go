package qa

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEngineRunsStandaloneCommandAndWritesEvidence(t *testing.T) {
	root := t.TempDir()
	result, err := Run(context.Background(), Request{
		Root:       root,
		Entrypoint: EntrypointStandalone,
		Mode:       ModeVerify,
		Phase:      "6",
		Commands: []CommandSpec{{
			ID:      "echo",
			Kind:    CheckCommand,
			Name:    "Echo verification",
			Command: "echo qa-ok",
			CWD:     ".",
		}},
	})
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if result.Status != StatusPass {
		t.Fatalf("status = %s, want %s: %+v", result.Status, StatusPass, result)
	}
	if result.SessionID == "" {
		t.Fatalf("session id is empty")
	}
	if result.EvidenceDir == "" {
		t.Fatalf("evidence dir is empty")
	}
	for _, path := range []string{
		filepath.Join(result.EvidenceDir, "request.json"),
		filepath.Join(result.EvidenceDir, "result.json"),
		filepath.Join(result.EvidenceDir, "summary.md"),
		filepath.Join(result.EvidenceDir, "logs", "echo.log"),
		filepath.Join(result.EvidenceDir, "checks", "echo.json"),
	} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("expected artifact %s: %v", path, err)
		}
	}
	logBody, err := os.ReadFile(filepath.Join(result.EvidenceDir, "logs", "echo.log"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(logBody), "qa-ok") {
		t.Fatalf("log does not contain command output: %s", string(logBody))
	}
}

func TestEngineClassifiesMissingPlaywrightAsBlockedForE2E(t *testing.T) {
	root := t.TempDir()
	result, err := Run(context.Background(), Request{
		Root:       root,
		Entrypoint: EntrypointStandalone,
		Mode:       ModeE2E,
		Phase:      "6",
	})
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if result.Status != StatusBlocked {
		t.Fatalf("status = %s, want %s: %+v", result.Status, StatusBlocked, result)
	}
	if len(result.Dependencies) == 0 {
		t.Fatalf("expected missing playwright dependency")
	}
	dep := result.Dependencies[0]
	if dep.Name != "playwright" || dep.Status != DependencyMissing || !dep.Required {
		t.Fatalf("unexpected dependency: %+v", dep)
	}
	if !strings.Contains(dep.InstallHint, "npx playwright install") {
		t.Fatalf("missing install hint: %+v", dep)
	}
}

func TestDetectFrameworkDiscoversMonorepoCommands(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "backend", "Cargo.toml"), "[package]\nname = \"api\"\nversion = \"0.1.0\"\n")
	mustWrite(t, filepath.Join(root, "frontend", "package.json"), `{"devDependencies":{"vitest":"latest"}}`)

	result := DetectFramework(root)
	if result.Framework != "monorepo" {
		t.Fatalf("framework = %q, want monorepo: %+v", result.Framework, result)
	}
	if len(result.Components) != 2 {
		t.Fatalf("components = %+v, want backend and frontend", result.Components)
	}
	if !contains(result.RunCommands, "cd backend && cargo test") {
		t.Fatalf("missing backend command: %+v", result.RunCommands)
	}
	if !contains(result.RunCommands, "cd frontend && npx vitest run") {
		t.Fatalf("missing frontend command: %+v", result.RunCommands)
	}
}

func mustWrite(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func contains(values []string, needle string) bool {
	for _, value := range values {
		if value == needle {
			return true
		}
	}
	return false
}
