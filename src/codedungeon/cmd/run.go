package cmd

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/spf13/cobra"

	"github.com/loldinis/codedungeon/internal/db"
	"github.com/loldinis/codedungeon/internal/osadapter"
	"github.com/loldinis/codedungeon/internal/provider"
)

func RunCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "run",
		Short: "Run a CodeDungeon workflow under autonomous custody",
		RunE:  runStartE,
	}
	addRunStartFlags(c)
	c.AddCommand(runStartCmd())
	c.AddCommand(runStatusCmd())
	c.AddCommand(runUnlockCmd())
	return c
}

func runStartCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "start",
		Short: "Start an autonomous full/lite/oneshot/rules workflow",
		RunE:  runStartE,
	}
	addRunStartFlags(c)
	return c
}

func runStartE(c *cobra.Command, _ []string) error {
	mode := selectedRunMode(c)
	prompt, _ := c.Flags().GetString("prompt")
	dryRun, _ := c.Flags().GetBool("dry-run")
	if mode == "invalid" {
		return EmitErr("multiple mode flags supplied", "use exactly one of --full, --lite, --oneshot, --auto, --rules")
	}
	if mode == "auto" {
		mode = autoSelectMode(prompt)
	}
	if mode != "rules" && strings.TrimSpace(prompt) == "" {
		return EmitErr("prompt required", "pass --prompt")
	}
	fmt.Printf("CODEDUNGEON_MODE_SELECTED: %s - %s\n", mode, modeReason(mode, prompt))

	root := currentProjectRoot()
	if mode != "rules" {
		if err := verifyGitHubPREnvironment(root); err != nil {
			return err
		}
	}

	s, err := OpenDB(c)
	if err != nil {
		return EmitErr(err.Error(), "")
	}
	defer s.Close()
	if err := s.Init(); err != nil {
		return EmitErr(err.Error(), "")
	}

	branch := ""
	if mode != "rules" {
		branch = "feat/" + slugifyFeature(prompt)
	}
	runID, err := s.CreateRun(&db.Run{
		Feature:     prompt,
		Branch:      branch,
		Mode:        strings.ToUpper(mode),
		ProjectMode: "SINGLE",
	})
	if err != nil {
		return EmitErr(err.Error(), "")
	}
	if err := prepareRunForMode(s, runID, mode); err != nil {
		return EmitErr(err.Error(), "")
	}
	token, err := randomHex(32)
	if err != nil {
		return EmitErr(err.Error(), "")
	}
	sessionID, err := randomHex(16)
	if err != nil {
		return EmitErr(err.Error(), "")
	}
	if err := s.InsertRunSession(db.RunSession{
		ID:          sessionID,
		RunID:       runID,
		Provider:    provider.Detect().Name(),
		Mode:        mode,
		TokenSHA256: hashSessionToken(token),
		Status:      "RUNNING",
	}); err != nil {
		return EmitErr(err.Error(), "")
	}
	_, _ = s.InsertRunEvent(db.RunEvent{RunID: runID, SessionID: sessionID, Event: "session_started", Detail: mode})

	childPrompt := autonomousChildPrompt(mode, prompt, branch)
	if dryRun {
		_ = s.UpdateRunSessionStatus(sessionID, "ABORTED", "dry-run")
		return EmitJSON(map[string]any{"ok": true, "dry_run": true, "run_id": runID, "session_id": sessionID, "mode": mode, "branch": branch, "prompt": childPrompt})
	}

	if err := executeProviderChild(root, mode, childPrompt, runID, sessionID, token); err != nil {
		_ = s.UpdateRunSessionStatus(sessionID, "FAILED", err.Error())
		_, _ = s.InsertRunEvent(db.RunEvent{RunID: runID, SessionID: sessionID, Event: "session_failed", Detail: err.Error()})
		return EmitErr("codedungeon runner failed: "+err.Error(), "")
	}
	if mode == "rules" {
		_ = s.UpdateRunSessionStatus(sessionID, "COMPLETED", "")
		_, _ = s.InsertRunEvent(db.RunEvent{RunID: runID, SessionID: sessionID, Event: "session_completed", Detail: mode})
		return EmitJSON(map[string]any{"ok": true, "run_id": runID, "session_id": sessionID, "status": "COMPLETED"})
	}
	report, err := renderFinalReport(root, runID, sessionID, token)
	if err != nil {
		_ = s.UpdateRunSessionStatus(sessionID, "FAILED", err.Error())
		_, _ = s.InsertRunEvent(db.RunEvent{RunID: runID, SessionID: sessionID, Event: "report_failed", Detail: err.Error()})
		return EmitErr("codedungeon final report failed: "+err.Error(), "")
	}
	_ = s.UpdateRunSessionStatus(sessionID, "READY_FOR_USER_REVIEW", "")
	_, _ = s.InsertRunEvent(db.RunEvent{RunID: runID, SessionID: sessionID, Event: "ready_for_user_review", Detail: branch})
	fmt.Print(report)
	return nil
}

func addRunStartFlags(c *cobra.Command) {
	c.Flags().Bool("full", false, "run full workflow")
	c.Flags().Bool("lite", false, "run lite workflow")
	c.Flags().Bool("oneshot", false, "run oneshot workflow")
	c.Flags().Bool("auto", false, "auto-select workflow")
	c.Flags().Bool("rules", false, "run Project Rules discovery")
	c.Flags().String("prompt", "", "workflow prompt")
	c.Flags().Bool("dry-run", false, "create and show runner plan without launching provider")
}

func prepareRunForMode(s *db.Store, runID int64, mode string) error {
	switch mode {
	case "lite", "oneshot":
		for _, phase := range db.CanonicalPhases() {
			if phase == "7" {
				break
			}
			if err := s.SetPhaseStatus(runID, phase, "SKIPPED", fmt.Sprintf("%s compact workflow; final gates use QA, review, PR, and report evidence", mode), nil); err != nil {
				return err
			}
		}
	}
	return nil
}

func runStatusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show latest autonomous runner session",
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
			sess, err := s.LatestRunSession(run.ID)
			if err != nil {
				return EmitErr(err.Error(), "")
			}
			return EmitJSON(map[string]any{"ok": true, "run": run, "session": sess})
		},
	}
}

func runUnlockCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "unlock",
		Short: "Abort a stale/crashed autonomous session",
		RunE: func(c *cobra.Command, _ []string) error {
			reason, _ := c.Flags().GetString("reason")
			if strings.TrimSpace(reason) == "" {
				return EmitErr("--reason is required", "")
			}
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
			sess, err := s.ActiveRunSession(run.ID)
			if err != nil {
				return EmitErr(err.Error(), "")
			}
			if sess == nil {
				return EmitJSON(map[string]any{"ok": true, "status": "no_active_session"})
			}
			if err := s.UpdateRunSessionStatus(sess.ID, "ABORTED", reason); err != nil {
				return EmitErr(err.Error(), "")
			}
			_, _ = s.InsertRunEvent(db.RunEvent{RunID: run.ID, SessionID: sess.ID, Event: "session_aborted", Detail: reason})
			return EmitJSON(map[string]any{"ok": true, "session_id": sess.ID, "status": "ABORTED"})
		},
	}
	c.Flags().String("reason", "", "why the session is being aborted")
	return c
}

func selectedRunMode(c *cobra.Command) string {
	flags := []struct {
		name string
		mode string
	}{
		{"full", "full"},
		{"lite", "lite"},
		{"oneshot", "oneshot"},
		{"auto", "auto"},
		{"rules", "rules"},
	}
	selected := "auto"
	count := 0
	for _, f := range flags {
		if ok, _ := c.Flags().GetBool(f.name); ok {
			selected = f.mode
			count++
		}
	}
	if count > 1 {
		return "invalid"
	}
	return selected
}

func autoSelectMode(prompt string) string {
	lower := strings.ToLower(prompt)
	if strings.Contains(lower, "plan") || strings.Contains(lower, "phase") || strings.Contains(lower, "multi") {
		return "full"
	}
	if strings.Contains(lower, ".codedungeon/plans") {
		return "lite"
	}
	return "oneshot"
}

func modeReason(mode, prompt string) string {
	switch mode {
	case "full":
		return "full phase lifecycle requested or selected"
	case "lite":
		return "planned lightweight workflow selected"
	case "oneshot":
		return "small direct workflow selected"
	case "rules":
		return "project rules discovery selected"
	default:
		return "automatic selection"
	}
}

func verifyGitHubPREnvironment(root string) error {
	if out, errb, err := run(root, "git", "remote", "get-url", "origin"); err != nil || strings.TrimSpace(out) == "" {
		return EmitErr("GitHub origin remote required", strings.TrimSpace(errb))
	}
	if _, errb, err := run(root, "gh", "auth", "status"); err != nil {
		return EmitErr("GitHub CLI authentication required", strings.TrimSpace(errb))
	}
	return nil
}

func randomHex(n int) (string, error) {
	buf := make([]byte, n)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}

func slugifyFeature(prompt string) string {
	lower := strings.ToLower(prompt)
	re := regexp.MustCompile(`[^a-z0-9]+`)
	slug := strings.Trim(re.ReplaceAllString(lower, "-"), "-")
	if slug == "" {
		return "codedungeon-run"
	}
	if len(slug) > 48 {
		slug = strings.Trim(slug[:48], "-")
	}
	return slug
}

func autonomousChildPrompt(mode, prompt, branch string) string {
	var skill string
	switch mode {
	case "full":
		skill = ".agents/skills/main-quest/SKILL.md"
	case "lite":
		skill = ".agents/skills/side-quest/SKILL.md"
	case "oneshot":
		skill = ".agents/skills/one-shot/SKILL.md"
	case "rules":
		skill = ".agents/skills/codedungeon/SKILL.md"
	}
	return fmt.Sprintf(`You are the CodeDungeon autonomous child runner.

Load and follow %s. The parent agent is no longer in control of workflow steps.

Required custody:
- Use only the project-local codedungeon binary for workflow evidence.
- A run and custody session already exist. Do not run phase init or create another run.
- Do not manually write review reports, final reports, persona evidence, or DB rows.
- Do not merge pull requests.
- Finish with the PR open for human review and merge.

Workflow mode: %s
Target branch: %s
User prompt:
%s
`, skill, mode, branch, prompt)
}

func executeProviderChild(root, mode, prompt string, runID int64, sessionID, token string) error {
	ad := osadapter.Detect()
	p := provider.Detect()
	env := append(os.Environ(),
		envRunID+"="+fmt.Sprintf("%d", runID),
		envSessionID+"="+sessionID,
		envSessionToken+"="+token,
	)
	var name string
	var args []string
	switch p.Name() {
	case "codex":
		resolved, err := ad.FindTool("codex")
		if err != nil {
			return err
		}
		name = resolved
		out := filepath.Join(root, provider.Detect().StateDir(), "runner-last-message.txt")
		args = []string{"exec", "--cd", root, "--dangerously-bypass-approvals-and-sandbox", "--enable", "multi_agent_v2", "--output-last-message", out, "-"}
	default:
		resolved, err := ad.FindTool("claude")
		if err != nil {
			return err
		}
		name = resolved
		args = []string{"--print", "--output-format", "stream-json", "--dangerously-skip-permissions", prompt}
		prompt = ""
	}
	cmd := exec.Command(name, args...)
	cmd.Dir = root
	cmd.Env = env
	if prompt != "" {
		cmd.Stdin = strings.NewReader(prompt)
	}
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func renderFinalReport(root string, runID int64, sessionID, token string) (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	cmd := exec.Command(exe, "report", "render")
	cmd.Dir = root
	cmd.Env = append(os.Environ(),
		envRunID+"="+fmt.Sprintf("%d", runID),
		envSessionID+"="+sessionID,
		envSessionToken+"="+token,
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("%w: %s", err, strings.TrimSpace(string(out)))
	}
	return string(out), nil
}
