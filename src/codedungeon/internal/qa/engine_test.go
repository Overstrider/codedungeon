package qa

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
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

func TestEngineE2EModeUsesPlaywrightComponentInMonorepo(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "backend", "Cargo.toml"), "[package]\nname = \"api\"\nversion = \"0.1.0\"\n")
	mustWrite(t, filepath.Join(root, "frontend", "package.json"), `{"devDependencies":{"@playwright/test":"latest","next":"latest"}}`)
	mustWrite(t, filepath.Join(root, "frontend", "playwright.config.ts"), "export default {}\n")
	prependFakeNpx(t)

	result, err := Run(context.Background(), Request{
		Root:       root,
		Entrypoint: EntrypointStandalone,
		Mode:       ModeE2E,
		Phase:      "6",
	})
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if result.Status != StatusPass {
		t.Fatalf("status = %s, want %s: %+v", result.Status, StatusPass, result)
	}
	if len(result.Dependencies) != 1 || result.Dependencies[0].Status != DependencyPresent {
		t.Fatalf("expected present playwright dependency: %+v", result.Dependencies)
	}
	if len(result.Checks) != 1 {
		t.Fatalf("checks = %+v, want one playwright check", result.Checks)
	}
	check := result.Checks[0]
	if check.Kind != CheckPlaywright || check.CWD != "frontend" {
		t.Fatalf("unexpected playwright check: %+v", check)
	}
	if !strings.Contains(check.Command, "--reporter=json") {
		t.Fatalf("playwright command should capture json evidence: %q", check.Command)
	}
}

func TestEnginePreflightOnlyDoesNotExecuteChecks(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "package.json"), `{"devDependencies":{"@playwright/test":"latest","next":"latest"}}`)
	mustWrite(t, filepath.Join(root, "playwright.config.ts"), "export default {}\n")
	prependFakeNpxWithTestExit(t, 7)

	result, err := Run(context.Background(), Request{
		Root:          root,
		Entrypoint:    EntrypointStandalone,
		Mode:          ModeE2E,
		Phase:         "6",
		PreflightOnly: true,
	})
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if result.Status != StatusPass {
		t.Fatalf("status = %s, want %s: %+v", result.Status, StatusPass, result)
	}
	if len(result.Checks) != 0 {
		t.Fatalf("preflight should not execute checks: %+v", result.Checks)
	}
	if len(result.Dependencies) != 1 || result.Dependencies[0].Status != DependencyPresent {
		t.Fatalf("expected present playwright dependency: %+v", result.Dependencies)
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

func prependFakeNpx(t *testing.T) {
	t.Helper()
	prependFakeNpxWithTestExit(t, 0)
}

func prependFakeNpxWithTestExit(t *testing.T, testExit int) {
	t.Helper()
	bin := t.TempDir()
	name := "npx"
	body := "#!/bin/sh\nif [ \"$1\" = \"playwright\" ] && [ \"$2\" = \"--version\" ]; then echo 'Version 1.99.0'; exit 0; fi\nif [ \"$1\" = \"playwright\" ]; then echo 'fake playwright ok'; exit " + fmt.Sprint(testExit) + "; fi\nexit 1\n"
	if runtime.GOOS == "windows" {
		name = "npx.cmd"
		body = "@echo off\r\nif \"%1\"==\"playwright\" (\r\n  if \"%2\"==\"--version\" (\r\n    echo Version 1.99.0\r\n    exit /b 0\r\n  )\r\n  echo fake playwright ok\r\n  exit /b " + fmt.Sprint(testExit) + "\r\n)\r\nexit /b 1\r\n"
	}
	path := filepath.Join(bin, name)
	if err := os.WriteFile(path, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" {
		if err := os.Chmod(path, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
}

func contains(values []string, needle string) bool {
	for _, value := range values {
		if value == needle {
			return true
		}
	}
	return false
}
