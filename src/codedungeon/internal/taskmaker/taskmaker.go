package taskmaker

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Request is the skill-authored, English input consumed by the renderer.
type Request struct {
	Title           string `json:"title"`
	Goal            string `json:"goal"`
	CurrentState    string `json:"current_state"`
	TargetOutcome   string `json:"target_outcome"`
	InScope         string `json:"in_scope"`
	OutOfScope      string `json:"out_of_scope"`
	Constraints     string `json:"constraints"`
	SuccessCriteria string `json:"success_criteria"`
	Verification    string `json:"verification"`
	Assumptions     string `json:"assumptions"`
}

// Result contains the rendered artifact bodies and their output directory.
type Result struct {
	OutDir string
	Design string
	Prompt string
	Output string
}

// Surface selects the provider-native command syntax printed in output.md.
type Surface string

const (
	SurfaceCodex  Surface = "codex"
	SurfaceClaude Surface = "claude"
)

// Render validates the request, renders standard task-maker artifacts, and
// writes request.json, design.md, prompt.txt, and output.md into outDir.
func Render(req Request, outDir string) (Result, error) {
	return RenderWithSurface(req, outDir, string(SurfaceCodex))
}

// RenderWithSurface validates the request, renders standard task-maker
// artifacts, and prints a provider-native run-full command for surface.
func RenderWithSurface(req Request, outDir string, surface string) (Result, error) {
	s, err := normalizeSurface(surface)
	if err != nil {
		return Result{}, err
	}
	req = normalize(req)
	if err := validate(req); err != nil {
		return Result{}, err
	}
	if strings.TrimSpace(outDir) == "" {
		return Result{}, fmt.Errorf("out directory is required")
	}
	design := renderDesign(req)
	prompt := renderPrompt(req)
	output := renderOutput(design, prompt, s)

	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return Result{}, err
	}
	requestJSON, err := json.MarshalIndent(req, "", "  ")
	if err != nil {
		return Result{}, err
	}
	files := map[string][]byte{
		"request.json": append(requestJSON, '\n'),
		"design.md":    []byte("# Minimal Design\n\n" + design),
		"prompt.txt":   []byte(prompt),
		"output.md":    []byte(output),
	}
	for name, body := range files {
		if err := os.WriteFile(filepath.Join(outDir, name), body, 0o644); err != nil {
			return Result{}, err
		}
	}
	return Result{OutDir: outDir, Design: design, Prompt: prompt, Output: output}, nil
}

func normalizeSurface(surface string) (Surface, error) {
	switch Surface(strings.ToLower(strings.TrimSpace(surface))) {
	case SurfaceCodex:
		return SurfaceCodex, nil
	case SurfaceClaude:
		return SurfaceClaude, nil
	default:
		return "", fmt.Errorf("surface must be codex or claude")
	}
}

func validate(req Request) error {
	if req.Goal == "" {
		return fmt.Errorf("goal is required")
	}
	if req.TargetOutcome == "" {
		return fmt.Errorf("target_outcome is required")
	}
	if req.SuccessCriteria == "" {
		return fmt.Errorf("success_criteria is required")
	}
	return nil
}

func normalize(req Request) Request {
	req.Title = clean(req.Title)
	req.Goal = clean(req.Goal)
	req.CurrentState = clean(req.CurrentState)
	req.TargetOutcome = clean(req.TargetOutcome)
	req.InScope = clean(req.InScope)
	req.OutOfScope = clean(req.OutOfScope)
	req.Constraints = clean(req.Constraints)
	req.SuccessCriteria = clean(req.SuccessCriteria)
	req.Verification = clean(req.Verification)
	req.Assumptions = clean(req.Assumptions)
	return req
}

func clean(s string) string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	return strings.TrimSpace(s)
}

func renderDesign(req Request) string {
	var b strings.Builder
	writeDesignField(&b, "Goal", req.Goal)
	writeDesignField(&b, "Current State", req.CurrentState)
	writeDesignField(&b, "Target Outcome", req.TargetOutcome)
	writeDesignField(&b, "In Scope", req.InScope)
	writeDesignField(&b, "Out of Scope", req.OutOfScope)
	writeDesignField(&b, "Constraints", req.Constraints)
	writeDesignField(&b, "Success Criteria", req.SuccessCriteria)
	writeDesignField(&b, "Verification", req.Verification)
	writeDesignField(&b, "Assumptions", req.Assumptions)
	return b.String()
}

func writeDesignField(b *strings.Builder, label, value string) {
	fmt.Fprintf(b, "%s:\n%s\n\n", label, valueOrDefault(value))
}

func valueOrDefault(value string) string {
	if strings.TrimSpace(value) == "" {
		return "None specified."
	}
	return value
}

func renderPrompt(req Request) string {
	parts := []string{
		clause("Deliver this target outcome: ", req.TargetOutcome),
		clause("Goal: ", req.Goal),
	}
	if req.CurrentState != "" {
		parts = append(parts, clause("Current state: ", req.CurrentState))
	}
	if req.InScope != "" {
		parts = append(parts, clause("In scope: ", req.InScope))
	}
	if req.OutOfScope != "" {
		parts = append(parts, clause("Out of scope: ", req.OutOfScope))
	}
	if req.Constraints != "" {
		parts = append(parts, clause("Constraints: ", req.Constraints))
	}
	parts = append(parts, clause("Acceptance criteria: ", req.SuccessCriteria))
	if req.Verification != "" {
		parts = append(parts, clause("Verification expected: ", req.Verification))
	}
	if req.Assumptions != "" {
		parts = append(parts, clause("Assumptions: ", req.Assumptions))
	}
	parts = append(parts, "If a missing decision would materially change the implementation, ask one concise clarification before proceeding; otherwise proceed with explicit assumptions and keep the change scoped to the stated outcome.")
	return strings.Join(parts, " ")
}

func clause(prefix, value string) string {
	value = strings.Join(strings.Fields(value), " ")
	if value == "" {
		return ""
	}
	if strings.HasSuffix(value, ".") || strings.HasSuffix(value, "!") || strings.HasSuffix(value, "?") {
		return prefix + value
	}
	return prefix + value + "."
}

func renderOutput(design, prompt string, surface Surface) string {
	var b strings.Builder
	b.WriteString("# Task Maker Output\n\n")
	b.WriteString("## Minimal Design\n")
	b.WriteString(design)
	b.WriteString("## Run Full Prompt\n")
	b.WriteString(prompt)
	b.WriteString("\n\n")
	b.WriteString("## Command\n")
	fmt.Fprintf(&b, "%s --full \"%s\"\n", commandForSurface(surface), escapeDoubleQuoted(prompt))
	return b.String()
}

func commandForSurface(surface Surface) string {
	if surface == SurfaceClaude {
		return "/codedungeon"
	}
	return "$codedungeon"
}

func escapeDoubleQuoted(s string) string {
	s = strings.ReplaceAll(s, "\\", "\\\\")
	return strings.ReplaceAll(s, "\"", "\\\"")
}
