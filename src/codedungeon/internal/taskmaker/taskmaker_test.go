package taskmaker

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRenderWritesArtifactsAndStandardOutput(t *testing.T) {
	out := t.TempDir()
	req := validRequest()

	result, err := Render(req, out)
	if err != nil {
		t.Fatal(err)
	}

	for _, name := range []string{"request.json", "design.md", "prompt.txt", "output.md"} {
		if _, err := os.Stat(filepath.Join(out, name)); err != nil {
			t.Fatalf("expected %s: %v", name, err)
		}
	}

	output, err := os.ReadFile(filepath.Join(out, "output.md"))
	if err != nil {
		t.Fatal(err)
	}
	if string(output) != result.Output {
		t.Fatalf("output.md did not match returned output")
	}
	for _, heading := range []string{
		"# Task Maker Output",
		"## Minimal Design",
		"Goal:",
		"Current State:",
		"Target Outcome:",
		"In Scope:",
		"Out of Scope:",
		"Constraints:",
		"Success Criteria:",
		"Verification:",
		"Assumptions:",
		"## Run Full Prompt",
		"## Command",
	} {
		if !strings.Contains(result.Output, heading) {
			t.Fatalf("output missing standard heading/label %q:\n%s", heading, result.Output)
		}
	}

	prompt, err := os.ReadFile(filepath.Join(out, "prompt.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(prompt) != result.Prompt {
		t.Fatalf("prompt.txt did not match returned prompt")
	}
	for _, required := range []string{
		req.TargetOutcome,
		req.SuccessCriteria,
		req.Verification,
		"ask one concise clarification",
		"proceed with explicit assumptions",
	} {
		if !strings.Contains(result.Prompt, required) {
			t.Fatalf("prompt missing %q:\n%s", required, result.Prompt)
		}
	}

	requestJSON, err := os.ReadFile(filepath.Join(out, "request.json"))
	if err != nil {
		t.Fatal(err)
	}
	var persisted Request
	if err := json.Unmarshal(requestJSON, &persisted); err != nil {
		t.Fatalf("request.json is invalid JSON: %v\n%s", err, requestJSON)
	}
	if persisted.Goal != req.Goal || persisted.TargetOutcome != req.TargetOutcome {
		t.Fatalf("persisted request = %+v, want %+v", persisted, req)
	}
}

func TestRenderRejectsMissingRequiredFields(t *testing.T) {
	tests := []struct {
		name string
		mut  func(*Request)
		want string
	}{
		{"goal", func(r *Request) { r.Goal = "  " }, "goal"},
		{"target", func(r *Request) { r.TargetOutcome = "" }, "target_outcome"},
		{"success", func(r *Request) { r.SuccessCriteria = "" }, "success_criteria"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := validRequest()
			tt.mut(&req)
			_, err := Render(req, t.TempDir())
			if err == nil {
				t.Fatal("Render succeeded, want validation error")
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error = %q, want it to mention %q", err, tt.want)
			}
		})
	}
}

func TestGeneratedCommandUsesCodedungeonFull(t *testing.T) {
	result, err := Render(validRequest(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result.Output, `$codedungeon --full "`) {
		t.Fatalf("command should use $codedungeon --full:\n%s", result.Output)
	}
	if strings.Contains(result.Output, "codedungeon run --full") {
		t.Fatalf("task-maker output should prepare the user-facing $codedungeon --full prompt, not dispatch run directly:\n%s", result.Output)
	}
}

func TestGeneratedCommandUsesRequestedSurface(t *testing.T) {
	tests := []struct {
		name      string
		surface   string
		want      string
		forbidden string
	}{
		{"codex", "codex", `$codedungeon --full "`, `/codedungeon --full "`},
		{"claude", "claude", `/codedungeon --full "`, `$codedungeon --full "`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := RenderWithSurface(validRequest(), t.TempDir(), tt.surface)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(result.Output, tt.want) {
				t.Fatalf("command should use %s surface:\n%s", tt.surface, result.Output)
			}
			if strings.Contains(result.Output, tt.forbidden) {
				t.Fatalf("command should not use other provider surface %q:\n%s", tt.forbidden, result.Output)
			}
		})
	}
}

func TestRenderRejectsInvalidSurface(t *testing.T) {
	_, err := RenderWithSurface(validRequest(), t.TempDir(), "browser")
	if err == nil {
		t.Fatal("RenderWithSurface succeeded, want validation error")
	}
	if !strings.Contains(err.Error(), "surface") {
		t.Fatalf("error = %q, want it to mention surface", err)
	}
}

func validRequest() Request {
	return Request{
		Title:           "Add export command",
		Goal:            "Let users export reports from the CLI.",
		CurrentState:    "Reports can be rendered but not exported.",
		TargetOutcome:   "Add a report export command that writes Markdown and JSON files.",
		InScope:         "CLI command, renderer wiring, focused tests, documentation.",
		OutOfScope:      "Changing report database schema or adding a web UI.",
		Constraints:     "Preserve existing report render behavior and keep output deterministic.",
		SuccessCriteria: "A user can run the export command and receives both files with stable names.",
		Verification:    "Run targeted command tests and go test ./...",
		Assumptions:     "Existing report data is available through the current store APIs.",
	}
}
