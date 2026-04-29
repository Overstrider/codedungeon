package codereview

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
)

type FilesRunner struct {
	InputDir string
}

func (r FilesRunner) RunPersona(_ context.Context, _ Request, persona string, outPath string) error {
	if r.InputDir == "" {
		return fmt.Errorf("input_dir is required for files runner")
	}
	src := filepath.Join(r.InputDir, "personas", persona+".json")
	body, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(outPath), 0o755); err != nil {
		return err
	}
	return os.WriteFile(outPath, body, 0o644)
}

func (r FilesRunner) RunAdjudicator(_ context.Context, _ Request, _ []PersonaReview, outPath string) error {
	if r.InputDir == "" {
		return fmt.Errorf("input_dir is required for files runner")
	}
	src := filepath.Join(r.InputDir, "review-decision.json")
	body, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(outPath), 0o755); err != nil {
		return err
	}
	return os.WriteFile(outPath, body, 0o644)
}
