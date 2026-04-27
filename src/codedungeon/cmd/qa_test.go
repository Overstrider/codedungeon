package cmd

import (
	"path/filepath"
	"testing"
)

func TestDetectTestFrameworkDiscoversMonorepoComponents(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "backend", "Cargo.toml"), `
[package]
name = "api"
version = "0.1.0"
`)
	writeFile(t, filepath.Join(root, "frontend", "package.json"), `{"devDependencies":{"vitest":"latest"}}`)

	result := detectProjectTestFramework(root)
	if result.Framework != "monorepo" {
		t.Fatalf("framework = %q, want monorepo: %+v", result.Framework, result)
	}
	if len(result.Components) != 2 {
		t.Fatalf("components = %+v, want backend and frontend", result.Components)
	}
	if result.RunCmd == "" {
		t.Fatalf("run_cmd should summarize component commands: %+v", result)
	}
	if !containsString(result.RunCmds, "cd backend && cargo test") {
		t.Fatalf("run_cmds missing backend cargo test: %+v", result.RunCmds)
	}
	if !containsString(result.RunCmds, "cd frontend && npx vitest run") {
		t.Fatalf("run_cmds missing frontend vitest: %+v", result.RunCmds)
	}
}
