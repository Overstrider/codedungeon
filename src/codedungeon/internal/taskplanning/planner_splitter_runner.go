package taskplanning

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

type PromptPlannerConfig struct {
	Enabled         bool     `json:"enabled"`
	Model           string   `json:"model"`
	ReasoningEffort string   `json:"reasoning_effort"`
	TimeoutSeconds  int      `json:"timeout_seconds"`
	Ephemeral       bool     `json:"ephemeral"`
	FallbackModels  []string `json:"fallback_models"`
}

func DefaultPromptPlannerConfig() PromptPlannerConfig {
	return PromptPlannerConfig{
		Enabled:         true,
		Model:           "gpt-5.5",
		ReasoningEffort: "high",
		TimeoutSeconds:  600,
		Ephemeral:       true,
		FallbackModels:  []string{"gpt-5.4"},
	}
}

type PromptPlannerRequest struct {
	Prompt          string
	ProjectContext  string
	WorkspacePolicy string
	OutputDir       string
	ProjectRules    ProjectRulesEnvelope
}

type PromptPlannerOutput struct {
	NeedsUserInput bool       `json:"needs_user_input"`
	Questions      []Question `json:"questions"`
	Summary        string     `json:"summary"`
	TaskGraph      *TaskGraph `json:"task_graph,omitempty"`
	Risks          []Risk     `json:"risks"`
}

type PromptPlanner interface {
	Plan(ctx context.Context, req PromptPlannerRequest) (PromptPlannerOutput, error)
}

type PlannerCommandInvocation struct {
	Name  string
	Args  []string
	Stdin string
}

type PlannerCommandRunner func(context.Context, PlannerCommandInvocation) error

type PlannerSplitterRunner struct {
	WorkDir         string
	Model           string
	ReasoningEffort string
	Timeout         time.Duration
	Ephemeral       bool
	FallbackModels  []string
	AvailableModels []string
	CommandRunner   PlannerCommandRunner
}

func NewPlannerSplitterRunner(workDir string, cfg PromptPlannerConfig) PlannerSplitterRunner {
	return PlannerSplitterRunner{
		WorkDir:         workDir,
		Model:           cfg.Model,
		ReasoningEffort: cfg.ReasoningEffort,
		Timeout:         time.Duration(cfg.TimeoutSeconds) * time.Second,
		Ephemeral:       cfg.Ephemeral,
		FallbackModels:  append([]string(nil), cfg.FallbackModels...),
	}
}

func (r PlannerSplitterRunner) Plan(ctx context.Context, req PromptPlannerRequest) (PromptPlannerOutput, error) {
	req.Prompt = strings.TrimSpace(req.Prompt)
	if req.Prompt == "" {
		return PromptPlannerOutput{}, fmt.Errorf("prompt is required")
	}
	if strings.TrimSpace(req.OutputDir) == "" {
		return PromptPlannerOutput{}, fmt.Errorf("output_dir is required")
	}
	if err := os.MkdirAll(req.OutputDir, 0o755); err != nil {
		return PromptPlannerOutput{}, err
	}
	schemaPath := filepath.Join(req.OutputDir, "planner-splitter.schema.json")
	if err := os.WriteFile(schemaPath, []byte(plannerSplitterOutputSchema()), 0o644); err != nil {
		return PromptPlannerOutput{}, err
	}
	lastMessagePath := filepath.Join(req.OutputDir, "planner-splitter-output.json")
	timeout := r.Timeout
	if timeout <= 0 {
		timeout = time.Duration(DefaultPromptPlannerConfig().TimeoutSeconds) * time.Second
	}
	timeoutCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	model := r.selectedModel()
	args := []string{"exec"}
	workDir := strings.TrimSpace(r.WorkDir)
	if workDir != "" {
		args = append(args, "--cd", workDir)
	}
	if r.Ephemeral {
		args = append(args, "--ephemeral")
	}
	args = append(args,
		"--sandbox", "read-only",
		"--model", model,
	)
	if effort := strings.TrimSpace(r.ReasoningEffort); effort != "" {
		args = append(args, "--config", fmt.Sprintf("model_reasoning_effort=%q", effort))
	}
	args = append(args,
		"--output-schema", schemaPath,
		"--output-last-message", lastMessagePath,
		"-",
	)
	commandRunner := r.CommandRunner
	if commandRunner == nil {
		commandRunner = defaultPlannerCommandRunner
	}
	if err := commandRunner(timeoutCtx, PlannerCommandInvocation{
		Name:  "codex",
		Args:  args,
		Stdin: plannerSplitterPrompt(req),
	}); err != nil {
		return PromptPlannerOutput{}, err
	}
	body, err := os.ReadFile(lastMessagePath)
	if err != nil {
		return PromptPlannerOutput{}, fmt.Errorf("read planner output: %w", err)
	}
	var out PromptPlannerOutput
	if err := json.Unmarshal(trimUTF8BOMBytes(body), &out); err != nil {
		return PromptPlannerOutput{}, fmt.Errorf("invalid planner JSON: %w", err)
	}
	return out, nil
}

func (r PlannerSplitterRunner) selectedModel() string {
	model := strings.TrimSpace(r.Model)
	if model == "" {
		model = DefaultPromptPlannerConfig().Model
	}
	if len(r.AvailableModels) == 0 || stringSliceContainsExact(r.AvailableModels, model) {
		return model
	}
	for _, fallback := range r.FallbackModels {
		fallback = strings.TrimSpace(fallback)
		if fallback != "" && stringSliceContainsExact(r.AvailableModels, fallback) {
			return fallback
		}
	}
	return model
}

func defaultPlannerCommandRunner(ctx context.Context, inv PlannerCommandInvocation) error {
	cmd := exec.CommandContext(ctx, inv.Name, inv.Args...)
	cmd.Stdin = strings.NewReader(inv.Stdin)
	cmd.Stdout = os.Stdout
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%s %s failed: %w: %s", inv.Name, strings.Join(inv.Args, " "), err, strings.TrimSpace(stderr.String()))
	}
	return nil
}

type PlannerSplitterFilesRunner struct {
	InputDir string
}

func (r PlannerSplitterFilesRunner) Plan(_ context.Context, req PromptPlannerRequest) (PromptPlannerOutput, error) {
	if strings.TrimSpace(r.InputDir) == "" {
		return PromptPlannerOutput{}, fmt.Errorf("planner files runner input dir is required")
	}
	body, err := os.ReadFile(filepath.Join(r.InputDir, "planner_splitter.json"))
	if err != nil {
		return PromptPlannerOutput{}, fmt.Errorf("read planner fixture: %w", err)
	}
	if strings.TrimSpace(req.OutputDir) != "" {
		if err := os.MkdirAll(req.OutputDir, 0o755); err != nil {
			return PromptPlannerOutput{}, err
		}
		_ = os.WriteFile(filepath.Join(req.OutputDir, "planner-splitter-output.json"), body, 0o644)
	}
	var out PromptPlannerOutput
	if err := json.Unmarshal(trimUTF8BOMBytes(body), &out); err != nil {
		return PromptPlannerOutput{}, fmt.Errorf("invalid planner JSON: %w", err)
	}
	return out, nil
}

func plannerSplitterPrompt(req PromptPlannerRequest) string {
	return fmt.Sprintf(`You are CodeDungeon planner+splitter.

Role:
- Do not edit files.
- Only produce JSON matching the provided schema.
- Search the project before assuming behavior is missing.
- Do not create placeholders, stubs, fake tests, or TODO-only implementation tasks.
- Split work by clear write scope.
- Include verification commands and risk notes for every task.
- Every task must have a non-empty write_scope. Verification-only tasks still need a narrow write_scope, such as relevant test, docs, or config files they validate.
- Use repo "." for the current project unless the prompt explicitly describes multiple repositories. Do not put absolute filesystem paths in repo.
- Use portable relative paths in write_scope.
- If the prompt is materially ambiguous, set needs_user_input=true and include concrete questions instead of inventing requirements.

User prompt:
%s

Project context:
%s

Workspace policy:
%s

Project rules:
PROJECT_RULES_STATUS: %s
PROJECT_RULES_DIGEST: %s
PROJECT_RULES_READ: %s

Return strict JSON with keys: needs_user_input, questions, summary, task_graph, risks.
`, req.Prompt, req.ProjectContext, req.WorkspacePolicy,
		fallback(req.ProjectRules.Status, "missing"), fallback(req.ProjectRules.Digest, "none"), fallback(req.ProjectRules.Read, "yes"))
}

func plannerSplitterOutputSchema() string {
	return `{
  "type": "object",
  "additionalProperties": false,
  "required": ["needs_user_input", "questions", "summary", "risks", "task_graph"],
  "properties": {
    "needs_user_input": {"type": "boolean"},
    "questions": {
      "type": "array",
      "items": {
        "type": "object",
        "additionalProperties": false,
        "required": ["question", "impact", "material"],
        "properties": {
          "question": {"type": "string"},
          "impact": {"type": "string"},
          "material": {"type": "boolean"}
        }
      }
    },
    "summary": {"type": "string"},
    "risks": {
      "type": "array",
      "items": {
        "type": "object",
        "additionalProperties": false,
        "required": ["title", "impact", "mitigation", "severity"],
        "properties": {
          "title": {"type": "string"},
          "impact": {"type": "string"},
          "mitigation": {"type": "string"},
          "severity": {"type": "string"}
        }
      }
    },
    "task_graph": {
      "anyOf": [
        {"type": "null"},
        {
          "type": "object",
          "additionalProperties": false,
          "required": ["version", "tasks"],
          "properties": {
            "version": {"type": "integer"},
            "tasks": {
              "type": "array",
              "items": {
                "type": "object",
                "additionalProperties": false,
                "required": ["id", "repo", "kind", "title", "objective", "context", "write_scope", "depends_on", "wave", "parallel_group", "owner_role", "acceptance_criteria", "verification_commands", "risk_notes"],
                "properties": {
                  "id": {"type": "string"},
                  "repo": {"type": "string"},
                  "kind": {"type": "string"},
                  "title": {"type": "string"},
                  "objective": {"type": "string"},
                  "context": {"type": "array", "items": {"type": "string"}},
                  "write_scope": {"type": "array", "items": {"type": "string"}},
                  "depends_on": {"type": "array", "items": {"type": "string"}},
                  "wave": {"type": "integer"},
                  "parallel_group": {"type": "string"},
                  "owner_role": {"type": "string"},
                  "acceptance_criteria": {"type": "array", "items": {"type": "string"}},
                  "verification_commands": {"type": "array", "items": {"type": "string"}},
                  "risk_notes": {"type": "array", "items": {"type": "string"}}
                }
              }
            }
          }
        }
      ]
    }
  }
}`
}

func trimUTF8BOMBytes(body []byte) []byte {
	if len(body) >= 3 && body[0] == 0xEF && body[1] == 0xBB && body[2] == 0xBF {
		return body[3:]
	}
	return body
}

func stringSliceContainsExact(values []string, want string) bool {
	for _, value := range values {
		if strings.TrimSpace(value) == want {
			return true
		}
	}
	return false
}
