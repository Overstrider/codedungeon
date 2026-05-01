package cmd

import (
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/loldinis/codedungeon/internal/db"
	"github.com/loldinis/codedungeon/internal/provider"
)

type diagnosticReport struct {
	OK           bool              `json:"ok"`
	Root         string            `json:"root"`
	Checks       []diagnosticCheck `json:"checks"`
	NextCommands []string          `json:"next_commands,omitempty"`
}

type diagnosticCheck struct {
	ID      string `json:"id"`
	Status  string `json:"status"`
	Detail  string `json:"detail,omitempty"`
	Blocker bool   `json:"blocker,omitempty"`
}

func DiagnoseCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "diagnose",
		Short: "Diagnose CodeDungeon binary-only readiness",
		RunE: func(c *cobra.Command, _ []string) error {
			report, err := buildDiagnostics(currentProjectRoot())
			if err != nil {
				return EmitErr(err.Error(), "")
			}
			return EmitJSON(report)
		},
	}
}

func buildDiagnostics(root string) (diagnosticReport, error) {
	report := diagnosticReport{OK: true, Root: root}
	add := func(id, status, detail string, blocker bool, next ...string) {
		report.Checks = append(report.Checks, diagnosticCheck{ID: id, Status: status, Detail: detail, Blocker: blocker})
		if blocker {
			report.OK = false
			report.NextCommands = append(report.NextCommands, next...)
		}
	}

	if _, err := os.Stat(filepath.Join(root, ".git")); err != nil {
		add("git", "FAIL", ".git not found", true, "git init")
	} else {
		add("git", "PASS", "git repository found", false)
	}

	dbPath := projectPath(root, provider.Detect().DBPath())
	if _, err := os.Stat(dbPath); err != nil {
		add("database", "FAIL", dbPath+" not found", true, "codedungeon phase init --feature <feature> --branch <branch> --project-mode SINGLE")
	} else {
		store, err := db.Open(dbPath)
		if err != nil {
			add("database", "FAIL", err.Error(), true, "codedungeon migrate")
		} else {
			defer store.Close()
			if err := store.Init(); err != nil {
				add("database", "FAIL", err.Error(), true, "codedungeon migrate")
			} else {
				add("database", "PASS", dbPath, false)
			}
		}
	}

	if firstExistingFile(
		filepath.Join(root, ".codedungeon", "project-context.md"),
		filepath.Join(root, ".codedungeon", "project-rules.compact.md"),
	) == "" {
		add("project_context", "FAIL", "no project context file found", true, "codedungeon project-context build")
	} else {
		add("project_context", "PASS", "project context found", false)
	}

	planPath := filepath.Join(root, ".codedungeon", "plan", "PLAN.md")
	taskMatches, _ := filepath.Glob(filepath.Join(root, ".codedungeon", "tasks", "task-*.md"))
	if !fileExists(planPath) || len(taskMatches) == 0 {
		add("planning_artifacts", "FAIL", "canonical PLAN.md or task-*.md files missing", true, "codedungeon plan run --prompt <task> --promote --auto-repair")
	} else {
		add("planning_artifacts", "PASS", planPath, false)
	}

	if !fileExists(filepath.Join(root, ".ralphrc")) {
		add("executor_config", "WARN", ".ralphrc not found; defaults will be used", false)
	} else {
		add("executor_config", "PASS", ".ralphrc found", false)
	}

	report.NextCommands = dedupeDiagnosticCommands(report.NextCommands)
	return report, nil
}

func firstExistingFile(paths ...string) string {
	for _, path := range paths {
		if fileExists(path) {
			return path
		}
	}
	return ""
}

func dedupeDiagnosticCommands(commands []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(commands))
	for _, command := range commands {
		if command == "" || seen[command] {
			continue
		}
		seen[command] = true
		out = append(out, command)
	}
	return out
}
