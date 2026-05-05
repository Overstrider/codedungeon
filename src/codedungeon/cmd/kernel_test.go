package cmd

import (
	"bytes"
	"encoding/json"
	"testing"
)

func TestKernelManifestReportsAgentWorkflowKernel(t *testing.T) {
	manifest := buildKernelManifest()

	if !manifest.OK {
		t.Fatal("kernel manifest should report ok")
	}
	if manifest.Name != "CodeDungeon Kernel" {
		t.Fatalf("manifest name = %q", manifest.Name)
	}
	if manifest.State.Database != ".codedungeon/codedungeon.db" {
		t.Fatalf("database path = %q", manifest.State.Database)
	}
	if manifest.State.Backend != "sqlite_fts5" {
		t.Fatalf("state backend = %q", manifest.State.Backend)
	}
	if !manifest.LocalProjectConfig {
		t.Fatal("kernel manifest should declare project-local configuration")
	}
	if manifest.License != "AGPL-3.0-only" {
		t.Fatalf("license = %q", manifest.License)
	}

	surfaces := map[string]string{}
	for _, surface := range manifest.ProviderSurfaces {
		surfaces[surface.Provider] = surface.Router
	}
	if surfaces["codex"] != "$codedungeon" {
		t.Fatalf("codex router surface = %q", surfaces["codex"])
	}
	if surfaces["claude"] != "/codedungeon" {
		t.Fatalf("claude router surface = %q", surfaces["claude"])
	}

	for _, want := range []string{"rules", "oneshot", "lite", "full"} {
		if !kernelHasMode(manifest, want) {
			t.Fatalf("missing workflow mode %q in %+v", want, manifest.Modes)
		}
	}
	for _, want := range []string{"project_rules", "task_maker", "artifact_registry", "git_guard_pr_verify", "qa", "code_review", "finalization"} {
		if !kernelHasModule(manifest, want) {
			t.Fatalf("missing kernel module %q in %+v", want, manifest.Modules)
		}
	}
	for _, want := range []string{"project_rules", "qa", "code_review", "github_pr", "artifact_integrity", "final_report"} {
		if !kernelHasGate(manifest, want) {
			t.Fatalf("missing completion gate %q in %+v", want, manifest.Gates)
		}
	}
}

func TestKernelCmdPrintsMachineReadableJSON(t *testing.T) {
	cmd := KernelCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)

	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}

	var payload kernelManifest
	if err := json.Unmarshal(out.Bytes(), &payload); err != nil {
		t.Fatalf("kernel output is not JSON: %v\n%s", err, out.String())
	}
	if payload.Name != "CodeDungeon Kernel" {
		t.Fatalf("payload name = %q", payload.Name)
	}
	if len(payload.ProviderSurfaces) < 2 {
		t.Fatalf("expected provider surfaces in payload: %+v", payload.ProviderSurfaces)
	}
}

func kernelHasMode(manifest kernelManifest, id string) bool {
	for _, mode := range manifest.Modes {
		if mode.ID == id {
			return true
		}
	}
	return false
}

func kernelHasModule(manifest kernelManifest, id string) bool {
	for _, module := range manifest.Modules {
		if module.ID == id {
			return true
		}
	}
	return false
}

func kernelHasGate(manifest kernelManifest, id string) bool {
	for _, gate := range manifest.Gates {
		if gate.ID == id {
			return true
		}
	}
	return false
}
