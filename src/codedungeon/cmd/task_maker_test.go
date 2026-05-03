package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestTaskMakerRenderCommandWritesArtifactsAndPrintsOutput(t *testing.T) {
	root := t.TempDir()
	input := filepath.Join(root, "request.json")
	outDir := filepath.Join(root, ".codedungeon", "task-maker", "sessions", "20260503-export-command")
	body := `{
  "title": "Add export command",
  "goal": "Let users export reports from the CLI.",
  "current_state": "Reports can be rendered but not exported.",
  "target_outcome": "Add a report export command that writes Markdown and JSON files.",
  "in_scope": "CLI command, renderer wiring, focused tests, documentation.",
  "out_of_scope": "Changing report database schema or adding a web UI.",
  "constraints": "Preserve existing report render behavior and keep output deterministic.",
  "success_criteria": "A user can run the export command and receives both files with stable names.",
  "verification": "Run targeted command tests and go test ./...",
  "assumptions": "Existing report data is available through the current store APIs."
}`
	if err := os.WriteFile(input, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}

	cmd := TaskMakerCmd()
	cmd.SetArgs([]string{"render", "--surface", "codex", "--input", input, "--out", outDir, "--print"})
	stdout, err := executeCommandInDir(root, cmd)
	if err != nil {
		t.Fatalf("task-maker render failed: %v\n%s", err, stdout)
	}

	for _, name := range []string{"request.json", "design.md", "prompt.txt", "output.md"} {
		assertFileExists(t, filepath.Join(outDir, name))
	}
	if !strings.Contains(stdout, "# Task Maker Output") ||
		!strings.Contains(stdout, "## Run Full Prompt") ||
		!strings.Contains(stdout, `$codedungeon --full "`) {
		t.Fatalf("unexpected stdout:\n%s", stdout)
	}
	output, err := os.ReadFile(filepath.Join(outDir, "output.md"))
	if err != nil {
		t.Fatal(err)
	}
	if stdout != string(output) {
		t.Fatalf("--print stdout should match output.md")
	}
}

func TestTaskMakerRenderCommandSupportsClaudeSurface(t *testing.T) {
	root := t.TempDir()
	input := filepath.Join(root, "request.json")
	outDir := filepath.Join(root, ".codedungeon", "task-maker", "sessions", "20260503-claude-command")
	if err := os.WriteFile(input, []byte(taskMakerRequestJSON()), 0o644); err != nil {
		t.Fatal(err)
	}

	cmd := TaskMakerCmd()
	cmd.SetArgs([]string{"render", "--surface", "claude", "--input", input, "--out", outDir, "--print"})
	stdout, err := executeCommandInDir(root, cmd)
	if err != nil {
		t.Fatalf("task-maker render failed: %v\n%s", err, stdout)
	}

	for _, name := range []string{"request.json", "design.md", "prompt.txt", "output.md"} {
		assertFileExists(t, filepath.Join(outDir, name))
	}
	if !strings.Contains(stdout, `/codedungeon --full "`) {
		t.Fatalf("stdout should contain Claude-native command:\n%s", stdout)
	}
	if strings.Contains(stdout, `$codedungeon --full "`) {
		t.Fatalf("stdout should not contain Codex command for Claude surface:\n%s", stdout)
	}
	output, err := os.ReadFile(filepath.Join(outDir, "output.md"))
	if err != nil {
		t.Fatal(err)
	}
	if stdout != string(output) {
		t.Fatalf("--print stdout should match output.md")
	}
}

func taskMakerRequestJSON() string {
	return `{
  "title": "Add export command",
  "goal": "Let users export reports from the CLI.",
  "current_state": "Reports can be rendered but not exported.",
  "target_outcome": "Add a report export command that writes Markdown and JSON files.",
  "in_scope": "CLI command, renderer wiring, focused tests, documentation.",
  "out_of_scope": "Changing report database schema or adding a web UI.",
  "constraints": "Preserve existing report render behavior and keep output deterministic.",
  "success_criteria": "A user can run the export command and receives both files with stable names.",
  "verification": "Run targeted command tests and go test ./...",
  "assumptions": "Existing report data is available through the current store APIs."
}`
}
