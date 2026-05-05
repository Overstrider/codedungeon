package cmd

import (
	"encoding/json"
	"fmt"
	"io"

	"github.com/spf13/cobra"

	"github.com/loldinis/codedungeon/internal/provider"
)

type kernelManifest struct {
	OK                 bool                    `json:"ok"`
	Name               string                  `json:"name"`
	Version            string                  `json:"version"`
	Description        string                  `json:"description"`
	CurrentProvider    string                  `json:"current_provider"`
	ProviderSurfaces   []kernelProviderSurface `json:"provider_surfaces"`
	Modes              []kernelMode            `json:"modes"`
	Workflow           []kernelWorkflowStage   `json:"workflow"`
	Modules            []kernelModule          `json:"modules"`
	Gates              []kernelGate            `json:"gates"`
	State              kernelState             `json:"state"`
	LocalProjectConfig bool                    `json:"local_project_config"`
	Principles         []string                `json:"principles"`
	License            string                  `json:"license"`
}

type kernelProviderSurface struct {
	Provider             string   `json:"provider"`
	Router               string   `json:"router"`
	TaskMaker            string   `json:"task_maker"`
	CodeReview           string   `json:"code_review"`
	CompatibilityAliases []string `json:"compatibility_aliases"`
	InstallPaths         []string `json:"install_paths"`
}

type kernelMode struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Command string `json:"command"`
	UseWhen string `json:"use_when"`
}

type kernelWorkflowStage struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Module      string `json:"module"`
	Evidence    string `json:"evidence"`
	Recoverable bool   `json:"recoverable"`
}

type kernelModule struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Commands    []string `json:"commands"`
	State       []string `json:"state"`
	Description string   `json:"description"`
}

type kernelGate struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Source      string `json:"source"`
	RequiredFor string `json:"required_for"`
}

type kernelState struct {
	RuntimeRoot      string   `json:"runtime_root"`
	Database         string   `json:"database"`
	Backend          string   `json:"backend"`
	ProviderDirs     []string `json:"provider_dirs"`
	MutableArtifacts []string `json:"mutable_artifacts"`
}

func KernelCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "kernel",
		Short: "Print the machine-readable CodeDungeon workflow kernel manifest",
		RunE: func(c *cobra.Command, _ []string) error {
			manifest := buildKernelManifest()
			if Human(c) {
				return writeKernelManifestHuman(c.OutOrStdout(), manifest)
			}
			enc := json.NewEncoder(c.OutOrStdout())
			enc.SetIndent("", "  ")
			enc.SetEscapeHTML(false)
			return enc.Encode(manifest)
		},
	}
}

func buildKernelManifest() kernelManifest {
	return kernelManifest{
		OK:              true,
		Name:            "CodeDungeon Kernel",
		Version:         versionString(),
		Description:     "Machine-to-machine workflow kernel for Codex and Claude Code agents.",
		CurrentProvider: provider.Detect().Name(),
		ProviderSurfaces: []kernelProviderSurface{
			{
				Provider:             "codex",
				Router:               "$codedungeon",
				TaskMaker:            "$task-maker",
				CodeReview:           "$code-review",
				CompatibilityAliases: []string{"$one-shot", "$side-quest", "$main-quest"},
				InstallPaths:         []string{".codex/bin/codedungeon", ".codex/agents", ".agents/skills", ".codedungeon/commands", ".codedungeon/phases"},
			},
			{
				Provider:             "claude",
				Router:               "/codedungeon",
				TaskMaker:            "/task-maker",
				CodeReview:           "/code-review",
				CompatibilityAliases: []string{"/one-shot", "/side-quest", "/main-quest"},
				InstallPaths:         []string{".claude/bin/codedungeon", ".claude/agents", ".claude/skills", ".claude/commands", ".codedungeon/commands", ".codedungeon/phases"},
			},
		},
		Modes: []kernelMode{
			{ID: "rules", Name: "Project Rules Discovery", Command: "$codedungeon --rules or /codedungeon --rules", UseWhen: "Deep-read a project, draft rules, wait for explicit approval, and compact shared context."},
			{ID: "oneshot", Name: "One Shot", Command: "codedungeon run --oneshot --prompt <prompt>", UseWhen: "Small scoped PR-producing changes without task splitting."},
			{ID: "lite", Name: "Side Quest", Command: "codedungeon run --lite --prompt <prompt>", UseWhen: "Simple planned work with an existing project plan."},
			{ID: "full", Name: "Main Quest", Command: "codedungeon run --full --prompt <prompt>", UseWhen: "Complex, multi-repo, architectural, QA-heavy, or final-report work."},
		},
		Workflow: []kernelWorkflowStage{
			{ID: "project_rules", Name: "Project Rules", Module: "project_rules", Evidence: ".codedungeon/project-rules.compact.md", Recoverable: true},
			{ID: "task_maker", Name: "Task Maker", Module: "task_maker", Evidence: ".codedungeon/task-maker/sessions/<session>/", Recoverable: true},
			{ID: "planning", Name: "Planning and Task Graph", Module: "planning", Evidence: ".codedungeon/plan/ and .codedungeon/tasks/", Recoverable: true},
			{ID: "execution", Name: "Execution", Module: "execution", Evidence: ".codedungeon/execute/sessions/<session>/", Recoverable: true},
			{ID: "qa", Name: "QA Verification", Module: "qa", Evidence: ".codedungeon/qa/sessions/<session>/", Recoverable: true},
			{ID: "code_review", Name: "Code Review", Module: "code_review", Evidence: ".codedungeon/code-review/", Recoverable: true},
			{ID: "finalization", Name: "Finalization", Module: "finalization", Evidence: ".codedungeon/reports/ and phase 7 DB evidence", Recoverable: false},
		},
		Modules: []kernelModule{
			{
				ID:          "project_rules",
				Name:        "Project Rules",
				Commands:    []string{"codedungeon rules status", "codedungeon rules lint", "codedungeon rules approve", "codedungeon rules compact", "codedungeon rules gate"},
				State:       []string{".codedungeon/project-rules.md", ".codedungeon/project-rules.compact.md", ".codedungeon/project-rules.json"},
				Description: "Shared approved project context for plans, tasks, reviews, handoffs, and final reports.",
			},
			{
				ID:          "task_maker",
				Name:        "Task Maker",
				Commands:    []string{"codedungeon task-maker render"},
				State:       []string{".codedungeon/task-maker/sessions/<session>/request.json", ".codedungeon/task-maker/sessions/<session>/design.md", ".codedungeon/task-maker/sessions/<session>/prompt.txt", ".codedungeon/task-maker/sessions/<session>/output.md"},
				Description: "Clarifies rough user intent and renders reviewed provider-native full-run prompts.",
			},
			{
				ID:          "planning",
				Name:        "Planning",
				Commands:    []string{"codedungeon plan run", "codedungeon plan promote", "codedungeon plan validate", "codedungeon plan status"},
				State:       []string{".codedungeon/plan/", ".codedungeon/tasks/"},
				Description: "Creates durable task graphs and promotes executable task context.",
			},
			{
				ID:          "execution",
				Name:        "Implementation Executor",
				Commands:    []string{"codedungeon execute task", "codedungeon execute plan", "codedungeon run --full", "codedungeon run --lite", "codedungeon run --oneshot"},
				State:       []string{".codedungeon/execute/sessions/<session>/", ".codedungeon/state/"},
				Description: "Runs task contracts through custody-aware worker sessions with verification evidence.",
			},
			{
				ID:          "qa",
				Name:        "QA",
				Commands:    []string{"codedungeon qa run --auto", "codedungeon qa run --cwd <repo>", "codedungeon qa status --latest", "codedungeon qa report --latest"},
				State:       []string{".codedungeon/qa/sessions/<session>/"},
				Description: "Records concrete test, build, E2E, and verification evidence.",
			},
			{
				ID:          "code_review",
				Name:        "Code Review",
				Commands:    []string{"codedungeon code-review --post"},
				State:       []string{".codedungeon/code-review/"},
				Description: "Runs adversarial review, adjudicates findings, and posts PR review evidence.",
			},
			{
				ID:          "artifact_registry",
				Name:        "Artifact Registry",
				Commands:    []string{"codedungeon artifacts list --latest-run", "codedungeon artifacts verify --latest-run", "codedungeon artifacts backfill --run <run-id>"},
				State:       []string{".codedungeon/codedungeon.db:artifacts"},
				Description: "Indexes runtime evidence so agents can verify files before relying on them.",
			},
			{
				ID:          "git_guard_pr_verify",
				Name:        "Git Guard and PR Verify",
				Commands:    []string{"codedungeon git guard", "codedungeon git pr", "codedungeon git verify"},
				State:       []string{"git remote", "GitHub PR state", ".codedungeon/codedungeon.db"},
				Description: "Keeps product workflows PR-centered and checks branch, push, and GitHub readiness.",
			},
			{
				ID:          "finalization",
				Name:        "Finalization",
				Commands:    []string{"codedungeon run finalize", "codedungeon report render"},
				State:       []string{".codedungeon/reports/", ".codedungeon/codedungeon.db"},
				Description: "Closes final phase state and emits READY_FOR_USER_REVIEW only after gates pass.",
			},
			{
				ID:          "telemetry",
				Name:        "Agent Telemetry",
				Commands:    []string{"codedungeon trace agent-start", "codedungeon trace agent-end", "codedungeon observe agents", "codedungeon observe report"},
				State:       []string{".codedungeon/codedungeon.db:agent_events"},
				Description: "Records informational phase, worker, reviewer, and specialist agent timelines.",
			},
		},
		Gates: []kernelGate{
			{ID: "project_rules", Name: "Approved Project Rules", Source: "codedungeon rules status", RequiredFor: "full and lite workflow planning"},
			{ID: "qa", Name: "QA Verification", Source: "codedungeon qa run", RequiredFor: "PR-producing workflow finalization"},
			{ID: "code_review", Name: "Code Review Approval", Source: "codedungeon code-review --post", RequiredFor: "READY_FOR_USER_REVIEW"},
			{ID: "github_pr", Name: "GitHub PR", Source: "codedungeon git verify", RequiredFor: "PR-producing workflow finalization"},
			{ID: "artifact_integrity", Name: "Artifact Integrity", Source: "codedungeon artifacts verify --latest-run", RequiredFor: "trusted runtime evidence"},
			{ID: "final_report", Name: "Final Report", Source: "codedungeon run finalize", RequiredFor: "READY_FOR_USER_REVIEW"},
			{ID: "custody", Name: "Custody Delivery", Source: "codedungeon run", RequiredFor: "preventing manual continuation after runner failure"},
		},
		State: kernelState{
			RuntimeRoot:      ".codedungeon",
			Database:         ".codedungeon/codedungeon.db",
			Backend:          "sqlite_fts5",
			ProviderDirs:     []string{".codex", ".agents", ".claude"},
			MutableArtifacts: []string{".codedungeon/commands", ".codedungeon/phases", ".codedungeon/tasks", ".codedungeon/plans", ".codedungeon/plan", ".codedungeon/state", ".codedungeon/qa", ".codedungeon/code-review", ".codedungeon/reports"},
		},
		LocalProjectConfig: true,
		Principles: []string{
			"agent_first_m2m",
			"provider_native_surfaces",
			"project_local_setup",
			"durable_sqlite_state",
			"pr_centered_delivery",
			"verification_over_approval_text",
		},
		License: "AGPL-3.0-only",
	}
}

func writeKernelManifestHuman(w io.Writer, manifest kernelManifest) error {
	_, err := fmt.Fprintf(w, "%s (%s)\n\nProvider surfaces:\n", manifest.Name, manifest.Version)
	if err != nil {
		return err
	}
	for _, surface := range manifest.ProviderSurfaces {
		if _, err := fmt.Fprintf(w, "- %s: %s, %s, %s\n", surface.Provider, surface.Router, surface.TaskMaker, surface.CodeReview); err != nil {
			return err
		}
	}
	if _, err := fmt.Fprintln(w, "\nModes:"); err != nil {
		return err
	}
	for _, mode := range manifest.Modes {
		if _, err := fmt.Fprintf(w, "- %s: %s\n", mode.ID, mode.Command); err != nil {
			return err
		}
	}
	if _, err := fmt.Fprintln(w, "\nGates:"); err != nil {
		return err
	}
	for _, gate := range manifest.Gates {
		if _, err := fmt.Fprintf(w, "- %s: %s (%s)\n", gate.ID, gate.Name, gate.Source); err != nil {
			return err
		}
	}
	_, err = fmt.Fprintf(w, "\nState: %s using %s\n", manifest.State.Database, manifest.State.Backend)
	return err
}
