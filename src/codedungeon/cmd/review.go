package cmd

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"

	artifactreg "github.com/loldinis/codedungeon/internal/artifacts"
	"github.com/loldinis/codedungeon/internal/db"
	"github.com/loldinis/codedungeon/internal/provider"
	"github.com/loldinis/codedungeon/internal/reviewpipe"
)

type reviewManifest struct {
	PersonasExpected []string `json:"personas_expected"`
	BaseSHA          string   `json:"base_sha"`
	HeadSHA          string   `json:"head_sha"`
	PRNumber         string   `json:"pr_number"`
	Timestamp        string   `json:"timestamp"`
}

func (m *reviewManifest) UnmarshalJSON(body []byte) error {
	var raw struct {
		PersonasExpected []string        `json:"personas_expected"`
		BaseSHA          string          `json:"base_sha"`
		HeadSHA          string          `json:"head_sha"`
		PRNumber         json.RawMessage `json:"pr_number"`
		Timestamp        string          `json:"timestamp"`
	}
	if err := json.Unmarshal(body, &raw); err != nil {
		return err
	}
	m.PersonasExpected = raw.PersonasExpected
	m.BaseSHA = raw.BaseSHA
	m.HeadSHA = raw.HeadSHA
	m.Timestamp = raw.Timestamp
	if len(raw.PRNumber) == 0 || string(raw.PRNumber) == "null" {
		m.PRNumber = ""
		return nil
	}
	var asString string
	if err := json.Unmarshal(raw.PRNumber, &asString); err == nil {
		m.PRNumber = strings.TrimSpace(asString)
		return nil
	}
	var asNumber json.Number
	if err := json.Unmarshal(raw.PRNumber, &asNumber); err == nil {
		m.PRNumber = asNumber.String()
		return nil
	}
	var asFloat float64
	if err := json.Unmarshal(raw.PRNumber, &asFloat); err == nil {
		m.PRNumber = strconv.FormatFloat(asFloat, 'f', -1, 64)
	}
	return nil
}

// persistFindings writes each finding into the findings table of the active
// run so FTS5 search picks them up. Best-effort: silent on error (findings are
// also written to review.json which is the authoritative snapshot).
func persistFindings(store *db.Store, findings []reviewpipe.Finding, cycle int) {
	if store == nil {
		return
	}
	run, err := store.CurrentRun()
	if err != nil || run == nil {
		return
	}
	if cycle <= 0 {
		if max, err := store.MaxFindingCycle(run.ID); err == nil {
			cycle = max + 1
		} else {
			cycle = 1
		}
	}
	for _, f := range findings {
		raw, _ := json.Marshal(f)
		_, _ = store.InsertFinding(db.Finding{
			RunID:          run.ID,
			Cycle:          cycle,
			Severity:       f.Severity,
			File:           f.File,
			LineStart:      f.LineStart,
			LineEnd:        f.LineEnd,
			Title:          f.Title,
			EvidenceQuote:  f.EvidenceQuote,
			FlaggedBy:      f.FlaggedBy,
			Actionable:     f.Actionable,
			DesignDecision: f.DesignDecision,
			Rationale:      f.ClassifierRationale,
			FullJSON:       string(raw),
		})
	}
}

func ReviewCmd() *cobra.Command {
	c := &cobra.Command{Use: "review", Short: "Adversarial review pipeline (dedupe→filter→classify→render→verdict)"}
	c.AddCommand(reviewRunCmd())
	c.AddCommand(reviewPostCmd())
	c.AddCommand(reviewContextPathsCmd())
	return c
}

func reviewRunCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "run",
		Short: "Run dedupe+filter+classify+render+verdict on a findings dir",
		Long: `Reads findings-<persona>.json from --dir, applies validator-*.json
filters, merges classifier-*.json, renders review.md + review.json, returns verdict.
Use --only STEP to re-run one stage (dedupe|filter|classify|render|verdict).`,
		RunE: func(c *cobra.Command, _ []string) error {
			dir, _ := c.Flags().GetString("dir")
			only, _ := c.Flags().GetString("only")
			nitCap, _ := c.Flags().GetInt("nit-cap")
			validatorModel, _ := c.Flags().GetString("validator-model")
			classifierModel, _ := c.Flags().GetString("classifier-model")
			stackSpec, _ := c.Flags().GetString("stack-specialist")
			cycle, _ := c.Flags().GetInt("cycle")

			if dir == "" {
				dir = projectPath(currentProjectRoot(), filepath.Join(provider.Detect().ReviewsDir(), "adv-review"))
			}
			if _, err := os.Stat(dir); err != nil {
				return EmitErr("findings dir not found: "+dir, "create it or pass --dir")
			}
			store, openErr := OpenDB(c) // best-effort; Render falls back to embedded when no project DB exists.
			if openErr != nil && isMigrationRequired(openErr) {
				return EmitErr(openErr.Error(), "run: codedungeon migrate")
			}
			if store != nil {
				defer store.Close()
				if run, rErr := store.CurrentRun(); rErr == nil && run != nil {
					if err := requireAutonomousCustody(store, run.ID, "review run"); err != nil {
						return err
					}
				}
			}
			manifest, manifestPath, err := loadAndValidateReviewManifest(dir)
			if err != nil {
				return EmitErr(err.Error(), "")
			}

			// ---- Step 6: dedupe + promote ----
			personaFindings, personas, err := reviewpipe.LoadPersonaFindings(dir)
			if err != nil {
				return EmitErr(err.Error(), "")
			}
			if err := validateManifestPersonas(dir, manifest.PersonasExpected, personas); err != nil {
				return EmitErr(err.Error(), "")
			}
			merged := reviewpipe.DedupeAndPromote(personaFindings)
			merged, suppressed := reviewpipe.ApplyNitCap(merged, nitCap)
			merged = reviewpipe.AssignIDs(merged)
			if finalReviewRun(only) && len(merged) == 0 {
				return EmitErr("legacy review run cannot approve empty findings; use standalone code-review with final adjudication", "run `codedungeon code-review --url <url> --project-context <path> --task-context <path>`")
			}
			writeJSON(filepath.Join(dir, "findings-merged.json"), merged)
			if only == "dedupe" {
				return EmitJSON(map[string]any{"ok": true, "step": "dedupe", "count": len(merged), "suppressed_nits": suppressed})
			}

			// ---- Step 7: validator filter ----
			validators, err := reviewpipe.LoadValidators(dir)
			if err != nil {
				return EmitErr(err.Error(), "")
			}
			if finalReviewRun(only) {
				if err := requireValidatorEvidence(merged, validators); err != nil {
					return EmitErr(err.Error(), "")
				}
			}
			merged, dropped := reviewpipe.ApplyValidators(merged, validators)
			writeJSON(filepath.Join(dir, "findings-validated.json"), merged)
			if only == "filter" {
				return EmitJSON(map[string]any{"ok": true, "step": "filter", "count": len(merged), "dropped": dropped})
			}

			// ---- Step 7.5: classifier merge ----
			classifiers, err := reviewpipe.LoadClassifiers(dir)
			if err != nil {
				return EmitErr(err.Error(), "")
			}
			if finalReviewRun(only) {
				if err := requireClassifierEvidence(merged, classifiers); err != nil {
					return EmitErr(err.Error(), "")
				}
			}
			merged = reviewpipe.ApplyClassifiers(merged, classifiers)
			writeJSON(filepath.Join(dir, "findings-classified.json"), merged)
			if only == "classify" {
				return EmitJSON(map[string]any{"ok": true, "step": "classify", "count": len(merged)})
			}

			// ---- Step 8: merge stack-specialist findings (if present) ----
			stack, err := reviewpipe.LoadStackFindings(dir)
			if err != nil {
				return EmitErr(err.Error(), "")
			}
			if finalReviewRun(only) && strings.TrimSpace(stackSpec) != "" {
				if err := requireStackEvidence(dir, stack); err != nil {
					return EmitErr(err.Error(), "")
				}
			}
			if len(stack) > 0 {
				// Stack findings are NEW — also classify them (classifier-stack-*.json).
				stack = reviewpipe.AssignIDs(append([]reviewpipe.Finding{}, stack...))
				stack = reviewpipe.ApplyClassifiers(stack, classifiers)
				merged = append(merged, stack...)
			}
			writeJSON(filepath.Join(dir, "findings-final.json"), merged)

			// ---- Step 9: render (DB-aware: user template overrides embedded) ----
			tally := reviewpipe.BuildTally(merged, dropped, suppressed)
			verdict := reviewpipe.Verdict(tally)
			validatorLabel := reviewStageMetadataLabel(validatorModel, len(validators))
			classifierLabel := reviewStageMetadataLabel(classifierModel, len(classifiers))
			md, rj, err := reviewpipe.Render(store, merged, tally, verdict, personas, validatorLabel, classifierLabel, stackSpec)
			if err != nil {
				return EmitErr(err.Error(), "")
			}
			if err := os.WriteFile(filepath.Join(dir, "review.md"), []byte(md), 0o644); err != nil {
				return EmitErr(err.Error(), "")
			}
			if err := writeJSON(filepath.Join(dir, "review.json"), rj); err != nil {
				return EmitErr(err.Error(), "")
			}
			// Persist findings into DB so FTS5 search + history work across runs.
			persistFindings(store, merged, cycle)
			if err := persistReviewEvidence(store, dir, manifestPath, manifest, rj); err != nil {
				return EmitErr(err.Error(), "")
			}
			if only == "render" {
				return EmitJSON(map[string]any{"ok": true, "step": "render", "path_md": filepath.Join(dir, "review.md"), "path_json": filepath.Join(dir, "review.json")})
			}

			// ---- Step 11: verdict ----
			return EmitJSON(map[string]any{
				"ok":          true,
				"verdict":     verdict,
				"tally":       tally,
				"review_md":   filepath.Join(dir, "review.md"),
				"review_json": filepath.Join(dir, "review.json"),
				"personas":    personas,
			})
		},
	}
	c.Flags().String("dir", "", "findings dir (default .codedungeon/reviews/adv-review)")
	c.Flags().String("only", "", "run only one step: dedupe|filter|classify|render|verdict")
	c.Flags().Int("nit-cap", 3, "max P2 findings before roll-up to suppressed count")
	c.Flags().String("validator-model", "sonnet-4.6", "label for review.json metadata")
	c.Flags().String("classifier-model", "sonnet-4.6", "label for review.json metadata")
	c.Flags().String("stack-specialist", "", "e.g. rust-specialist (optional)")
	c.Flags().Int("cycle", 0, "adversarial review cycle (0 = auto-detect as MAX(cycle)+1)")
	return c
}

func finalReviewRun(only string) bool {
	return strings.TrimSpace(only) == ""
}

const noFindingsReviewStageLabel = "not_applicable_no_findings"

func reviewStageMetadataLabel(configured string, evidenceCount int) string {
	if evidenceCount == 0 {
		return noFindingsReviewStageLabel
	}
	return strings.TrimSpace(configured)
}

func requireValidatorEvidence(findings []reviewpipe.Finding, validators []reviewpipe.ValidatorResult) error {
	if len(findings) == 0 {
		return nil
	}
	byID := map[string]bool{}
	byIdx := map[int]bool{}
	for _, v := range validators {
		if v.FindingID != "" {
			byID[v.FindingID] = true
		}
		if v.Idx > 0 {
			byIdx[v.Idx] = true
		}
		if strings.TrimSpace(v.Confidence) == "" {
			return fmt.Errorf("validator evidence missing confidence")
		}
	}
	for i, f := range findings {
		if f.ID != "" && byID[f.ID] {
			continue
		}
		if byIdx[i+1] {
			continue
		}
		return fmt.Errorf("missing validator evidence for finding %s", firstNonEmpty(f.ID, fmt.Sprintf("#%d", i+1)))
	}
	return nil
}

func requireClassifierEvidence(findings []reviewpipe.Finding, classifiers []reviewpipe.ClassifierResult) error {
	if len(findings) == 0 {
		return nil
	}
	byID := map[string]bool{}
	byIdx := map[int]bool{}
	for _, c := range classifiers {
		if c.FindingID != "" {
			byID[c.FindingID] = true
		}
		if c.Idx > 0 {
			byIdx[c.Idx] = true
		}
		if strings.TrimSpace(c.Classification) == "" || strings.TrimSpace(c.Confidence) == "" {
			return fmt.Errorf("classifier evidence missing classification or confidence")
		}
	}
	for i, f := range findings {
		if f.ID != "" && byID[f.ID] {
			continue
		}
		if byIdx[i+1] {
			continue
		}
		return fmt.Errorf("missing classifier evidence for finding %s", firstNonEmpty(f.ID, fmt.Sprintf("#%d", i+1)))
	}
	return nil
}

func requireStackEvidence(dir string, stack []reviewpipe.Finding) error {
	path := filepath.Join(dir, "findings-stack.json")
	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("missing stack-specialist evidence: findings-stack.json")
		}
		return fmt.Errorf("read stack-specialist evidence: %w", err)
	}
	if len(stack) == 0 {
		return nil
	}
	paths, err := filepath.Glob(filepath.Join(dir, "classifier-stack-*.json"))
	if err != nil {
		return err
	}
	if len(paths) < len(stack) {
		return fmt.Errorf("missing stack classifier evidence: have %d, want %d", len(paths), len(stack))
	}
	return nil
}

func loadAndValidateReviewManifest(dir string) (reviewManifest, string, error) {
	path := filepath.Join(dir, "review-manifest.json")
	body, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return reviewManifest{}, path, fmt.Errorf("review-manifest.json required in %s", dir)
		}
		return reviewManifest{}, path, fmt.Errorf("read review-manifest.json: %w", err)
	}
	var m reviewManifest
	if err := json.Unmarshal(body, &m); err != nil {
		return reviewManifest{}, path, fmt.Errorf("unmarshal review-manifest.json: %w", err)
	}
	if len(m.PersonasExpected) == 0 {
		return reviewManifest{}, path, fmt.Errorf("review-manifest.json missing personas_expected")
	}
	if m.BaseSHA == "" || m.HeadSHA == "" {
		return reviewManifest{}, path, fmt.Errorf("review-manifest.json requires base_sha and head_sha")
	}
	if m.PRNumber == "" {
		return reviewManifest{}, path, fmt.Errorf("review-manifest.json requires pr_number")
	}
	if m.Timestamp == "" {
		return reviewManifest{}, path, fmt.Errorf("review-manifest.json requires timestamp")
	}
	if _, err := time.Parse(time.RFC3339, m.Timestamp); err != nil {
		return reviewManifest{}, path, fmt.Errorf("review-manifest.json timestamp must be RFC3339: %w", err)
	}
	m.PersonasExpected = uniqueSorted(m.PersonasExpected)
	return m, path, nil
}

func validateManifestPersonas(dir string, expected, loaded []string) error {
	if len(loaded) == 0 {
		return fmt.Errorf("no persona outputs found in %s", dir)
	}
	loadedSet := map[string]bool{}
	for _, p := range loaded {
		loadedSet[reviewpipe.CanonicalPersona(p)] = true
	}
	for _, persona := range expected {
		persona = reviewpipe.CanonicalPersona(persona)
		if !loadedSet[persona] {
			return fmt.Errorf("missing persona output: findings-%s.json", persona)
		}
		path, ok := reviewpipe.PersonaFindingsPath(dir, persona)
		if !ok {
			return fmt.Errorf("missing persona output: findings-%s.json", persona)
		}
		body, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("read %s: %w", path, err)
		}
		var pf reviewpipe.PersonaFile
		if err := json.Unmarshal(body, &pf); err != nil {
			var arr []reviewpipe.Finding
			if aerr := json.Unmarshal(body, &arr); aerr != nil {
				return fmt.Errorf("unmarshal %s: %w", path, err)
			}
			pf = reviewpipe.PersonaFile{Persona: persona, Findings: arr}
		}
		if len(pf.Findings) == 0 {
			if strings.TrimSpace(pf.NoFindingsRationale) == "" {
				return fmt.Errorf("%s has zero findings but no no_findings_rationale", path)
			}
			if pf.ReviewedFiles <= 0 {
				return fmt.Errorf("%s has zero findings but reviewed_files is zero", path)
			}
		}
	}
	return nil
}

func persistReviewEvidence(store *db.Store, dir, manifestPath string, manifest reviewManifest, review reviewpipe.ReviewJSON) error {
	if store == nil {
		return nil
	}
	run, err := store.CurrentRun()
	if err != nil || run == nil {
		return err
	}
	id, err := store.InsertReviewEvidence(db.ReviewEvidence{
		RunID:            run.ID,
		ReviewDir:        dir,
		ReviewJSONPath:   filepath.Join(dir, "review.json"),
		ManifestPath:     manifestPath,
		Verdict:          review.Verdict,
		PRNumber:         manifest.PRNumber,
		BaseSHA:          manifest.BaseSHA,
		HeadSHA:          manifest.HeadSHA,
		PersonasExpected: manifest.PersonasExpected,
		PersonasRun:      uniqueSorted(review.PersonasRun),
	})
	if err != nil {
		return err
	}
	registry := artifactreg.NewRegistry(store, currentProjectRoot())
	meta := map[string]any{"verdict": review.Verdict, "pr_number": manifest.PRNumber}
	for _, item := range []struct {
		role string
		kind string
		path string
	}{
		{"directory", "directory", dir},
		{"review_md", "markdown", filepath.Join(dir, "review.md")},
		{"review_json", "json", filepath.Join(dir, "review.json")},
		{"manifest", "json", manifestPath},
		{"findings", "json", filepath.Join(dir, "findings-final.json")},
	} {
		if err := artifactreg.RegisterIfExists(registry, artifactreg.Record{
			RunID: run.ID, Module: "review", OwnerType: "review_evidence", OwnerID: strconv.FormatInt(id, 10),
			Phase: "5.5", Role: item.role, Kind: item.kind, Path: item.path, Metadata: meta,
		}); err != nil {
			return err
		}
	}
	return nil
}

func uniqueSorted(in []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, v := range in {
		v = reviewpipe.CanonicalPersona(v)
		if v == "" || seen[v] {
			continue
		}
		seen[v] = true
		out = append(out, v)
	}
	sort.Strings(out)
	return out
}

func reviewPostCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "post",
		Short: "Post generated review.md to the GitHub PR and record comment custody",
		RunE: func(c *cobra.Command, _ []string) error {
			dir, _ := c.Flags().GetString("dir")
			s, err := OpenDB(c)
			if err != nil {
				return EmitErr(err.Error(), "")
			}
			defer s.Close()
			runRow, err := s.CurrentRun()
			if err != nil {
				return EmitErr(err.Error(), "")
			}
			if runRow == nil {
				return EmitErr("no active run", "")
			}
			if err := requireAutonomousCustody(s, runRow.ID, "review post"); err != nil {
				return err
			}
			evidence, err := s.LatestReviewEvidence(runRow.ID)
			if err != nil {
				return EmitErr(err.Error(), "")
			}
			if err := validateReviewEvidence(evidence); err != nil {
				return EmitErr("review post gate: "+err.Error(), "")
			}
			if evidence.ReviewDir == "" {
				return EmitErr("review post gate: review evidence missing directory", "")
			}
			if dir != "" && !samePath(dir, evidence.ReviewDir) {
				return EmitErr("review post gate: --dir does not match latest review evidence", evidence.ReviewDir)
			}
			bodyBytes, err := os.ReadFile(filepath.Join(evidence.ReviewDir, "review.md"))
			if err != nil {
				return EmitErr("read review.md: "+err.Error(), "")
			}
			body := string(bodyBytes)
			marker := provider.Detect().ReviewCommentMarker()
			if !strings.Contains(body, marker) {
				body = "## " + marker + "\n\n" + body
			}
			repoName, errb, err := run(".", "gh", "repo", "view", "--json", "nameWithOwner", "-q", ".nameWithOwner")
			if err != nil {
				return EmitErr("gh repo view failed: "+errb, "")
			}
			input, err := os.CreateTemp("", "codedungeon-review-*.json")
			if err != nil {
				return EmitErr(err.Error(), "")
			}
			defer os.Remove(input.Name())
			payload, _ := json.Marshal(map[string]string{"body": body})
			if _, err := input.Write(payload); err != nil {
				_ = input.Close()
				return EmitErr(err.Error(), "")
			}
			_ = input.Close()
			out, errb, err := run(".", "gh", "api", "-X", "POST",
				fmt.Sprintf("/repos/%s/issues/%s/comments", strings.TrimSpace(repoName), evidence.PRNumber),
				"--input", input.Name())
			if err != nil {
				return EmitErr("gh api review comment failed: "+errb, "")
			}
			var posted struct {
				ID      int64  `json:"id"`
				HTMLURL string `json:"html_url"`
				User    struct {
					Login string `json:"login"`
				} `json:"user"`
			}
			if err := json.Unmarshal([]byte(out), &posted); err != nil {
				return EmitErr("parse posted comment: "+err.Error(), "")
			}
			sum := sha256.Sum256([]byte(strings.TrimSpace(body)))
			id, err := s.InsertPRReviewPost(db.PRReviewPost{
				RunID:            runRow.ID,
				ReviewEvidenceID: evidence.ID,
				PRNumber:         evidence.PRNumber,
				CommentID:        fmt.Sprintf("%d", posted.ID),
				CommentURL:       posted.HTMLURL,
				BodySHA256:       hex.EncodeToString(sum[:]),
				PostedBy:         posted.User.Login,
			})
			if err != nil {
				return EmitErr(err.Error(), "")
			}
			return EmitJSON(map[string]any{"ok": true, "id": id, "pr_number": evidence.PRNumber, "comment_url": posted.HTMLURL})
		},
	}
	c.Flags().String("dir", "", "review dir (default .codedungeon/reviews/adv-review)")
	return c
}

func samePath(a, b string) bool {
	aa, errA := filepath.Abs(filepath.Clean(a))
	bb, errB := filepath.Abs(filepath.Clean(b))
	if errA == nil {
		a = aa
	}
	if errB == nil {
		b = bb
	}
	return strings.EqualFold(filepath.Clean(a), filepath.Clean(b))
}

func reviewContextPathsCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "context-paths",
		Short: "Resolve CLAUDE.md/REVIEW.md/ARCHITECTURE.md/ADRs/spec for classifier input",
		RunE: func(c *cobra.Command, _ []string) error {
			repo, _ := c.Flags().GetString("repo")
			if repo == "" {
				repo = "."
			}
			absRepo, _ := filepath.Abs(repo)
			projectRoot := ResolveProjectRoot(absRepo)

			existsOr := func(p string) string {
				if _, err := os.Stat(p); err == nil {
					return p
				}
				return "NONE"
			}
			adrs := "NONE"
			for _, d := range []string{"docs/adrs", "docs/adr"} {
				cand := filepath.Join(absRepo, d)
				if _, err := os.Stat(cand); err == nil {
					adrs = cand
					break
				}
			}
			p := provider.Detect()
			var taskFiles []string
			matches, _ := filepath.Glob(filepath.Join(projectRoot, p.TasksDir(), "**", "task-*.md"))
			taskFiles = append(taskFiles, matches...)

			return EmitJSON(map[string]any{
				"ok":                true,
				"project_root":      projectRoot,
				"repo":              absRepo,
				"agent_config_root": existsOr(filepath.Join(projectRoot, p.AgentConfigFile())),
				"agent_config_repo": existsOr(filepath.Join(absRepo, p.AgentConfigFile())),
				"review_md":         existsOr(filepath.Join(absRepo, "REVIEW.md")),
				"architecture_md":   firstExisting(filepath.Join(absRepo, "ARCHITECTURE.md"), filepath.Join(absRepo, "docs", "ARCHITECTURE.md")),
				"adr_paths":         adrs,
				"spec_md":           existsOr(filepath.Join(absRepo, "docs", "spec.md")),
				"task_files":        taskFiles,
			})
		},
	}
	c.Flags().String("repo", ".", "repo dir")
	return c
}

func firstExisting(paths ...string) string {
	for _, p := range paths {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return "NONE"
}

func writeJSON(path string, v any) error {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, b, 0o644)
}
