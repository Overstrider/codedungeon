package cmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"text/template"

	"github.com/spf13/cobra"

	"github.com/loldinis/codedungeon/internal/codereview"
	"github.com/loldinis/codedungeon/internal/db"
	"github.com/loldinis/codedungeon/internal/provider"
)

var _ = json.RawMessage{}

func PhaseCmd() *cobra.Command {
	c := &cobra.Command{Use: "phase", Short: "Phase lifecycle (state + handoff, atomic)"}
	c.AddCommand(phaseInitCmd())
	c.AddCommand(phaseStartCmd())
	c.AddCommand(phaseDoneCmd())
	c.AddCommand(phaseSkipCmd())
	c.AddCommand(phaseFailCmd())
	c.AddCommand(phaseNextCmd())
	c.AddCommand(phaseInfoCmd())
	c.AddCommand(phaseConfigCmd())
	c.AddCommand(phaseRenderStateCmd())
	return c
}

func phaseInitCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "init",
		Short: "Create a new pipeline run (10 phases seeded PENDING)",
		RunE: func(c *cobra.Command, _ []string) error {
			feature, _ := c.Flags().GetString("feature")
			branch, _ := c.Flags().GetString("branch")
			mode, _ := c.Flags().GetString("mode")
			projectMode, _ := c.Flags().GetString("project-mode")
			repoMapFile, _ := c.Flags().GetString("repo-map")
			if feature == "" {
				return EmitErr("--feature is required", "")
			}
			s, err := OpenDB(c)
			if err != nil {
				return EmitErr(err.Error(), "run `codedungeon db init` first")
			}
			defer s.Close()
			// Require schema to exist; init if not.
			if err := s.Init(); err != nil {
				return EmitErr(err.Error(), "")
			}
			if sess, err := s.ActiveAnyRunSession(); err != nil {
				return EmitErr(err.Error(), "")
			} else if sess != nil {
				return EmitErr("autonomous session already owns run state", "do not run phase init during codedungeon run; use the existing run or codedungeon run unlock for a stale session")
			}

			var repoMap json.RawMessage
			if repoMapFile != "" {
				b, err := os.ReadFile(repoMapFile)
				if err != nil {
					return EmitErr(err.Error(), "")
				}
				repoMap = b
			}

			r := &db.Run{
				Feature:     feature,
				Branch:      branch,
				Mode:        mode,
				ProjectMode: projectMode,
				RepoMap:     repoMap,
			}
			id, err := s.CreateRun(r)
			if err != nil {
				return EmitErr(err.Error(), "")
			}
			return EmitJSON(map[string]any{"ok": true, "run_id": id, "phases_seeded": db.CanonicalPhases()})
		},
	}
	c.Flags().String("feature", "", "feature prompt (required)")
	c.Flags().String("branch", "", "feature branch")
	c.Flags().String("mode", "FRESH", "FRESH | APPEND")
	c.Flags().String("project-mode", "", "SINGLE | MULTI | BOOTSTRAP")
	c.Flags().String("repo-map", "", "path to JSON file with repo map")
	return c
}

func phaseStartCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "start <phase>",
		Short: "Mark phase IN_PROGRESS + stamp started_at",
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			return setStatusRun(c, args[0], "IN_PROGRESS", "", nil, nil).err
		},
	}
}

// phaseTerminal is the shared implementation for done/skip/fail — the three
// terminal states that also write a handoff.
func phaseTerminal(status string) *cobra.Command {
	verb := strings.ToLower(status)
	c := &cobra.Command{
		Use:   verb + " <phase>",
		Short: fmt.Sprintf("Mark phase %s + write handoff atomically", status),
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			phase := args[0]
			summary, _ := c.Flags().GetString("summary")
			decisions, _ := c.Flags().GetStringArray("decisions")
			artifacts, _ := c.Flags().GetStringArray("artifacts")
			traps, _ := c.Flags().GetStringArray("traps")
			questions, _ := c.Flags().GetStringArray("questions")
			nextInput, _ := c.Flags().GetString("next")
			promise, _ := c.Flags().GetString("promise")
			reason, _ := c.Flags().GetString("reason")
			writeFile, _ := c.Flags().GetString("write-file")
			notes, _ := c.Flags().GetString("notes")

			// For skip/fail, reason doubles as summary if summary not given.
			if summary == "" && reason != "" {
				summary = reason
			}
			// Default promise for skip/fail.
			if promise == "" {
				switch status {
				case "SKIPPED":
					promise = fmt.Sprintf("PHASE_%s_SKIPPED: %s", phaseLabel(phase), reason)
				case "FAIL":
					promise = fmt.Sprintf("PHASE_%s_FAILED: %s", phaseLabel(phase), reason)
				}
			}
			if notes == "" {
				notes = summary
			}

			// Verdict gate: phases 5 and 7 require --verdict for DONE.
			var gatedPhases = map[string]bool{"5": true, "7": true}
			if status == "DONE" && gatedPhases[phase] {
				verdict, _ := c.Flags().GetString("verdict")
				if phase == "5" {
					if verdict == "" {
						return EmitErr("--verdict is required for `phase done 5`",
							"pass --verdict APPROVED or --verdict CHANGES_REQUESTED")
					}
					if verdict != "APPROVED" {
						forceEsc, _ := c.Flags().GetString("force-escalate")
						if forceEsc == "" {
							return EmitErr("phase-5-not-approved",
								"run fix loop OR --force-escalate \"human-triage: <reason>\"")
						}
						status = "FAIL"
						if promise == "" {
							promise = fmt.Sprintf("PHASE_%s_FAILED: %s", phaseLabel(phase), forceEsc)
						}
						notes = "ESCALATED: " + forceEsc
					} else if err := validatePhase5Gate(c); err != nil {
						return err
					}
				}
				if phase == "7" {
					forceReport, _ := c.Flags().GetBool("force-report")
					if !forceReport {
						gs, gErr := OpenDB(c)
						if gErr == nil {
							run, rErr := gs.CurrentRun()
							if rErr == nil && run != nil {
								phases, _ := gs.AllPhases(run.ID)
								for _, p := range phases {
									if p.Phase == "7" {
										break
									}
									if p.Status == "FAIL" {
										gs.Close()
										return EmitErr("phase-7-blocked: phase "+p.Phase+" is FAIL",
											"fix upstream phase or pass --force-report")
									}
								}
							}
							gs.Close()
						}
					}
					if err := validatePhase7DoneGate(c); err != nil {
						return err
					}
				}
			}

			if status == "DONE" && promise == "" {
				return EmitErr("--promise is required for `codedungeon phase done`", "e.g. --promise 'PHASE_5_COMPLETE: ...'")
			}
			if status == "DONE" {
				if err := validatePhaseDoneArtifacts(c, phase, artifacts); err != nil {
					return err
				}
				if phase == "6" {
					if err := validatePhase6Gate(c); err != nil {
						return err
					}
				}
			}

			return setStatusRun(c, phase, status, notes, artifacts, &db.Handoff{
				Phase:         phase,
				Summary:       summary,
				Decisions:     decisions,
				Artifacts:     artifacts,
				Traps:         traps,
				OpenQuestions: questions,
				NextInput:     nextInput,
				Promise:       promise,
			}).orWriteFile(writeFile)
		},
	}
	c.Flags().String("summary", "", "1-line summary (required for DONE; optional for SKIP/FAIL if --reason given)")
	c.Flags().StringArray("decisions", nil, "key decisions (repeatable)")
	c.Flags().StringArray("artifacts", nil, "artifacts produced (paths, repeatable)")
	c.Flags().StringArray("traps", nil, "traps/warnings for next phase (repeatable)")
	c.Flags().StringArray("questions", nil, "open questions (repeatable)")
	c.Flags().String("next", "", "next phase input (paths)")
	c.Flags().String("promise", "", "canonical promise line (last line of handoff)")
	c.Flags().String("reason", "", "reason (for skip/fail)")
	c.Flags().String("notes", "", "override notes column in phase_status (default: summary)")
	c.Flags().String("write-file", "", "also write handoff markdown to this path")
	c.Flags().String("verdict", "", "review verdict (APPROVED|CHANGES_REQUESTED), required for phases 5/7")
	c.Flags().String("force-escalate", "", "override non-APPROVED verdict on phase 5 (escalation reason)")
	c.Flags().Bool("force-report", false, "force phase-7 report despite upstream FAIL phases")
	return c
}

func validatePhaseDoneArtifacts(c *cobra.Command, phase string, artifacts []string) error {
	switch phase {
	case "1", "2'":
	default:
		return nil
	}
	if len(artifacts) == 0 {
		return EmitErr("phase-"+phaseLabel(phase)+"-gate: at least one artifact path is required", "")
	}
	for _, artifact := range artifacts {
		path := strings.TrimSpace(artifact)
		if path == "" {
			continue
		}
		if strings.HasPrefix(path, "http://") || strings.HasPrefix(path, "https://") {
			return EmitErr("phase-"+phaseLabel(phase)+"-gate: artifact must be a local file: "+path, "")
		}
		if !filepath.IsAbs(path) {
			path = projectPath(currentProjectRoot(), path)
		}
		info, err := os.Stat(path)
		if err != nil {
			return EmitErr("phase-"+phaseLabel(phase)+"-gate: artifact not found: "+artifact, "")
		}
		if info.IsDir() || info.Size() == 0 {
			return EmitErr("phase-"+phaseLabel(phase)+"-gate: artifact is empty: "+artifact, "")
		}
	}
	return nil
}

func validatePhase5Gate(c *cobra.Command) error {
	s, err := OpenDB(c)
	if err != nil {
		return EmitErr(err.Error(), "")
	}
	defer s.Close()
	run, err := s.CurrentRun()
	if err != nil {
		return EmitErr(err.Error(), "")
	}
	if run == nil {
		return EmitErr("phase-5-gate: no active run", "")
	}
	evidence, err := s.LatestReviewEvidence(run.ID)
	if err != nil {
		return EmitErr(err.Error(), "")
	}
	if err := validateReviewEvidence(evidence); err != nil {
		return EmitErr("phase-5-gate: "+err.Error(), "")
	}
	if err := validateBranchPushed(run.Branch); err != nil {
		return EmitErr("phase-5-gate: "+err.Error(), "")
	}
	return nil
}

func validateReviewEvidence(e *db.ReviewEvidence) error {
	if e == nil {
		return fmt.Errorf("approved review evidence is required")
	}
	if e.Verdict != "APPROVED" {
		return fmt.Errorf("latest review evidence verdict is %s", e.Verdict)
	}
	if e.PRNumber == "" {
		return fmt.Errorf("review evidence missing PR number")
	}
	if e.BaseSHA == "" || e.HeadSHA == "" {
		return fmt.Errorf("review evidence missing base/head SHA")
	}
	if len(e.PersonasExpected) == 0 || len(e.PersonasRun) == 0 {
		return fmt.Errorf("review evidence missing persona outputs")
	}
	runSet := map[string]bool{}
	for _, persona := range e.PersonasRun {
		runSet[persona] = true
	}
	for _, persona := range e.PersonasExpected {
		if !runSet[persona] {
			return fmt.Errorf("review evidence missing persona %s", persona)
		}
	}
	result, err := codereview.ValidateResultDir(e.ReviewDir)
	if err != nil {
		return err
	}
	if result.Verdict != e.Verdict {
		return fmt.Errorf("review result verdict %s does not match evidence %s", result.Verdict, e.Verdict)
	}
	if err := validateStandaloneReviewMarkdown(e.ReviewDir); err != nil {
		return err
	}
	return nil
}

func validateReviewStageMetadata(field, value string) error {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return fmt.Errorf("review.json %s is required", field)
	}
	if strings.EqualFold(trimmed, "SKIPPED") {
		return fmt.Errorf("review.json %s cannot be SKIPPED in final review evidence", field)
	}
	return nil
}

func validateStandaloneReviewMarkdown(reviewDir string) error {
	if strings.TrimSpace(reviewDir) == "" {
		return fmt.Errorf("review evidence missing review directory")
	}
	body, err := os.ReadFile(filepath.Join(reviewDir, "review.md"))
	if err != nil {
		return fmt.Errorf("review.md not readable: %w", err)
	}
	text := string(body)
	for _, required := range []string{"CodeDungeon Code Review", "Verdict", "Review Integrity", "Findings", "Review Summary"} {
		if !strings.Contains(text, required) {
			return fmt.Errorf("review.md missing standalone review section %q", required)
		}
	}
	if strings.Contains(text, "_None._") {
		return fmt.Errorf("review.md contains empty review marker")
	}
	if strings.Contains(text, "Persona Approvals") || strings.Contains(text, "#### ") {
		return fmt.Errorf("review.md contains verbose persona report sections")
	}
	return nil
}

func validateBranchPushed(branch string) error {
	if branch == "" {
		return fmt.Errorf("run branch is required")
	}
	current, err := currentBranch(".")
	if err != nil {
		return err
	}
	if current != branch {
		return fmt.Errorf("current branch %s does not match run branch %s", current, branch)
	}
	upstream, errb, err := run(".", "git", "rev-parse", "--abbrev-ref", "--symbolic-full-name", "@{u}")
	if err != nil || strings.TrimSpace(upstream) == "" {
		return fmt.Errorf("branch is not pushed to an upstream: %s", strings.TrimSpace(errb))
	}
	unpushed, errb, err := run(".", "git", "rev-list", "--count", "@{u}..HEAD")
	if err != nil {
		return fmt.Errorf("cannot verify pushed branch: %s", strings.TrimSpace(errb))
	}
	if strings.TrimSpace(unpushed) != "0" {
		return fmt.Errorf("branch has unpushed commits")
	}
	return nil
}

func validatePhase6Gate(c *cobra.Command) error {
	s, err := OpenDB(c)
	if err != nil {
		return EmitErr(err.Error(), "")
	}
	defer s.Close()
	run, err := s.CurrentRun()
	if err != nil {
		return EmitErr(err.Error(), "")
	}
	if run == nil {
		return EmitErr("phase-6-gate: no active run", "")
	}
	records, err := s.VerificationRecords(run.ID, "6")
	if err != nil {
		return EmitErr(err.Error(), "")
	}
	if len(records) == 0 {
		return EmitErr("phase-6-gate: verification ledger is required", "record commands with `codedungeon qa run --phase 6 --cmd \"...\"`")
	}
	latest := latestVerificationRecords(records)
	if len(latest) == 0 {
		return EmitErr("phase-6-gate: verification ledger is required", "record commands with `codedungeon qa run --phase 6 --cmd \"...\"`")
	}
	for _, record := range latest {
		if record.Status != "PASS" {
			return EmitErr("phase-6-gate: verification command failed: "+record.Command, record.LogPath)
		}
		info, err := os.Stat(record.LogPath)
		if err != nil {
			return EmitErr("phase-6-gate: verification log not found: "+record.LogPath, "")
		}
		if info.Size() == 0 {
			return EmitErr("phase-6-gate: verification log is empty: "+record.LogPath, "")
		}
	}
	return nil
}

func validatePhase7DoneGate(c *cobra.Command) error {
	s, err := OpenDB(c)
	if err != nil {
		return EmitErr(err.Error(), "")
	}
	defer s.Close()
	run, err := s.CurrentRun()
	if err != nil {
		return EmitErr(err.Error(), "")
	}
	if run == nil {
		return EmitErr("phase-7-gate: no active run", "")
	}
	if err := validateReportGates(s, run, true, true); err != nil {
		return EmitErr("phase-7-gate: "+err.Error(), "")
	}
	return nil
}

// orWriteFile is chained on the EmitJSON return of setStatusRun; when writeFile
// is non-empty, dump the rendered handoff markdown too.
type runResult struct {
	err      error
	phase    string
	runID    int64
	rendered string
}

func (r runResult) orWriteFile(p string) error {
	if r.err != nil {
		return r.err
	}
	if p != "" && r.rendered != "" {
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			return EmitErr(err.Error(), "")
		}
		if err := os.WriteFile(p, []byte(r.rendered), 0o644); err != nil {
			return EmitErr(err.Error(), "")
		}
	}
	return nil
}

func phaseDoneCmd() *cobra.Command {
	return phaseTerminal("DONE")
}
func phaseSkipCmd() *cobra.Command {
	c := phaseTerminal("SKIPPED")
	c.Use = "skip <phase>"
	return c
}
func phaseFailCmd() *cobra.Command {
	c := phaseTerminal("FAIL")
	c.Use = "fail <phase>"
	return c
}

// setStatusRun is the atomic "update phase status + optionally write handoff +
// render markdown to .codedungeon/state/phase-{N}-output.md".
// Returns a runResult that EmitsJSON on success and lets caller chain file
// writes.
func setStatusRun(c *cobra.Command, phase, status, notes string, artifacts []string, h *db.Handoff) runResult {
	s, err := OpenDB(c)
	if err != nil {
		return runResult{err: EmitErr(err.Error(), "")}
	}
	defer s.Close()
	run, err := s.CurrentRun()
	if err != nil {
		return runResult{err: EmitErr(err.Error(), "")}
	}
	if run == nil {
		return runResult{err: EmitErr("no active run — run `codedungeon phase init` first", "")}
	}
	if err := requireAutonomousCustody(s, run.ID, "phase state mutation"); err != nil {
		return runResult{err: err}
	}
	if err := s.SetPhaseStatus(run.ID, phase, status, notes, artifacts); err != nil {
		return runResult{err: EmitErr(err.Error(), "")}
	}
	var rendered string
	if h != nil {
		h.RunID = run.ID
		h.Phase = phase
		rendered = renderHandoff(h, status)
		h.RenderedMD = rendered
		if err := s.UpsertHandoff(h); err != nil {
			return runResult{err: EmitErr(err.Error(), "")}
		}
		// Also write to .codedungeon/state/phase-{N}-output.md next to cwd.
		outPath := projectPath(currentProjectRoot(), filepath.Join(provider.Detect().StateDir(), "phase-"+phaseLabel(phase)+"-output.md"))
		_ = os.MkdirAll(filepath.Dir(outPath), 0o755)
		_ = os.WriteFile(outPath, []byte(rendered), 0o644)
	}
	_ = EmitJSON(map[string]any{
		"ok":         true,
		"run_id":     run.ID,
		"phase":      phase,
		"status":     status,
		"handoff_md": rendered != "",
	})
	return runResult{phase: phase, runID: run.ID, rendered: rendered}
}

// phaseLabel produces the filesystem-safe phase slug: 2' → 2prime, 3.5 → 35.
func phaseLabel(p string) string {
	r := strings.ReplaceAll(p, "'", "prime")
	r = strings.ReplaceAll(r, ".", "")
	return r
}

// renderHandoff formats the handoff into the canonical schema.
func renderHandoff(h *db.Handoff, status string) string {
	var b bytes.Buffer
	fmt.Fprintf(&b, "# phase-%s-output\n\n", phaseLabel(h.Phase))
	fmt.Fprintf(&b, "Phase: %s\n", h.Phase)
	fmt.Fprintf(&b, "Status: %s\n", status)
	if h.Summary != "" {
		fmt.Fprintf(&b, "Summary: %s\n\n", h.Summary)
	} else {
		b.WriteString("\n")
	}
	if len(h.Decisions) > 0 {
		b.WriteString("Key Decisions:\n")
		for _, d := range h.Decisions {
			fmt.Fprintf(&b, "- %s\n", d)
		}
		b.WriteString("\n")
	}
	if len(h.Artifacts) > 0 {
		b.WriteString("Artifacts Produced:\n")
		for _, a := range h.Artifacts {
			fmt.Fprintf(&b, "- %s\n", a)
		}
		b.WriteString("\n")
	}
	if len(h.Traps) > 0 {
		b.WriteString("Traps:\n")
		for _, t := range h.Traps {
			fmt.Fprintf(&b, "- %s\n", t)
		}
		b.WriteString("\n")
	}
	if len(h.OpenQuestions) > 0 {
		b.WriteString("Open Questions:\n")
		for _, q := range h.OpenQuestions {
			fmt.Fprintf(&b, "- %s\n", q)
		}
		b.WriteString("\n")
	}
	if h.NextInput != "" {
		fmt.Fprintf(&b, "Next Phase Input: %s\n\n", h.NextInput)
	}
	if h.Promise != "" {
		fmt.Fprintf(&b, "%s\n", h.Promise)
	}
	return b.String()
}

func phaseNextCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "next",
		Short: "Return the first non-DONE/SKIPPED phase",
		RunE: func(c *cobra.Command, _ []string) error {
			s, err := OpenDB(c)
			if err != nil {
				return EmitErr(err.Error(), "")
			}
			defer s.Close()
			run, err := s.CurrentRun()
			if err != nil {
				return EmitErr(err.Error(), "")
			}
			if run == nil {
				return EmitErr("no active run", "run `codedungeon phase init` first")
			}
			n, err := s.NextPending(run.ID)
			if err != nil {
				return EmitErr(err.Error(), "")
			}
			return EmitJSON(map[string]any{"ok": true, "run_id": run.ID, "next_phase": n, "done": n == ""})
		},
	}
}

func phaseInfoCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "info <phase>",
		Short: "Show phase status + handoff",
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			s, err := OpenDB(c)
			if err != nil {
				return EmitErr(err.Error(), "")
			}
			defer s.Close()
			run, err := s.CurrentRun()
			if err != nil {
				return EmitErr(err.Error(), "")
			}
			if run == nil {
				return EmitErr("no active run", "")
			}
			p, err := s.GetPhase(run.ID, args[0])
			if err != nil {
				return EmitErr(err.Error(), "")
			}
			h, _ := s.GetHandoff(run.ID, args[0])
			field, _ := c.Flags().GetString("field")
			if field != "" && h != nil {
				return emitHandoffField(h, field)
			}
			return EmitJSON(map[string]any{"ok": true, "phase": p, "handoff": h})
		},
	}
	c.Flags().String("field", "", "emit only this handoff field (summary|promise|next_input|decisions|artifacts|traps|rendered_md)")
	return c
}

func emitHandoffField(h *db.Handoff, field string) error {
	switch field {
	case "summary":
		fmt.Print(h.Summary)
	case "promise":
		fmt.Print(h.Promise)
	case "next_input":
		fmt.Print(h.NextInput)
	case "rendered_md":
		fmt.Print(h.RenderedMD)
	case "decisions":
		return EmitJSON(h.Decisions)
	case "artifacts":
		return EmitJSON(h.Artifacts)
	case "traps":
		return EmitJSON(h.Traps)
	case "questions":
		return EmitJSON(h.OpenQuestions)
	default:
		return EmitErr("unknown field: "+field, "")
	}
	return nil
}

func phaseConfigCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "config <key>",
		Short: "Get run-level config value (feature|branch|project_mode|mode)",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			s, err := OpenDB(c)
			if err != nil {
				return EmitErr(err.Error(), "")
			}
			defer s.Close()
			run, err := s.CurrentRun()
			if err != nil {
				return EmitErr(err.Error(), "")
			}
			if run == nil {
				return EmitErr("no active run", "")
			}
			if len(args) == 0 {
				return EmitJSON(run)
			}
			key := args[0]
			val := map[string]string{
				"feature":      run.Feature,
				"branch":       run.Branch,
				"project_mode": run.ProjectMode,
				"mode":         run.Mode,
			}[key]
			if val == "" {
				return EmitErr("unknown or empty key: "+key, "valid: feature, branch, project_mode, mode")
			}
			fmt.Print(val)
			return nil
		},
	}
	return c
}

func phaseRenderStateCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "render-state",
		Short: "Regenerate .codedungeon/plan/pipeline-state.md from DB",
		RunE: func(c *cobra.Command, _ []string) error {
			s, err := OpenDB(c)
			if err != nil {
				return EmitErr(err.Error(), "")
			}
			defer s.Close()
			run, err := s.CurrentRun()
			if err != nil {
				return EmitErr(err.Error(), "")
			}
			if run == nil {
				return EmitErr("no active run", "")
			}
			phases, err := s.AllPhases(run.ID)
			if err != nil {
				return EmitErr(err.Error(), "")
			}
			body := renderPipelineStateMD(run, phases)
			out, _ := c.Flags().GetString("out")
			if out == "" {
				out = projectPath(currentProjectRoot(), filepath.Join(provider.Detect().PlanDir(), "pipeline-state.md"))
			}
			_ = os.MkdirAll(filepath.Dir(out), 0o755)
			if err := os.WriteFile(out, []byte(body), 0o644); err != nil {
				return EmitErr(err.Error(), "")
			}
			return EmitJSON(map[string]any{"ok": true, "path": out, "bytes": len(body)})
		},
	}
	c.Flags().String("out", "", "override path (default .codedungeon/plan/pipeline-state.md)")
	return c
}

// renderPipelineStateMD produces a minimal, human-readable view; not intended
// to be the source-of-truth (the DB is). Phase files can still `Read` this.
func renderPipelineStateMD(run *db.Run, phases []db.Phase) string {
	tpl := `# Pipeline State

## Config
feature: {{.Feature}}
mode: {{.Mode}}
project_mode: {{.ProjectMode}}
branch: {{.Branch}}

## Phase Status
| Phase | Status | Artifacts | Notes |
|-------|--------|-----------|-------|
{{range .Phases -}}
| {{.Phase}} | {{.Status}} | {{join .Artifacts ", "}} | {{.Notes}} |
{{end}}
`
	t := template.Must(template.New("state").Funcs(template.FuncMap{
		"join": strings.Join,
	}).Parse(tpl))
	var b bytes.Buffer
	_ = t.Execute(&b, map[string]any{
		"Feature":     run.Feature,
		"Mode":        run.Mode,
		"ProjectMode": run.ProjectMode,
		"Branch":      run.Branch,
		"Phases":      phases,
	})
	return b.String()
}
