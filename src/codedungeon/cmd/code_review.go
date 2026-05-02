package cmd

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	artifactreg "github.com/loldinis/codedungeon/internal/artifacts"
	"github.com/loldinis/codedungeon/internal/codereview"
	"github.com/loldinis/codedungeon/internal/db"
	"github.com/loldinis/codedungeon/internal/provider"
)

func CodeReviewCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "code-review",
		Short: "Run standalone end-to-end CodeDungeon code review",
		RunE: func(c *cobra.Command, _ []string) error {
			url, _ := c.Flags().GetString("url")
			projectContextArg, _ := c.Flags().GetString("project-context")
			taskContextArg, _ := c.Flags().GetString("task-context")
			targetContextArg, _ := c.Flags().GetString("target-context")
			targetContextMode, _ := c.Flags().GetString("target-context-mode")
			maxTargetContextBytes, _ := c.Flags().GetInt("max-target-context-bytes")
			outDir, _ := c.Flags().GetString("out")
			runnerName, _ := c.Flags().GetString("runner")
			inputDir, _ := c.Flags().GetString("input-dir")
			post, _ := c.Flags().GetBool("post")
			rulesStatus, _ := c.Flags().GetString("project-rules-status")
			rulesDigest, _ := c.Flags().GetString("project-rules-digest")
			rulesRead, _ := c.Flags().GetString("project-rules-read")

			if outDir == "" {
				outDir = filepath.Join(currentProjectRoot(), ".codedungeon", "code-review")
			}
			projectContext, err := readContextArg(projectContextArg)
			if err != nil {
				return EmitErr("project-context: "+err.Error(), "")
			}
			taskContext, err := readContextArg(taskContextArg)
			if err != nil {
				return EmitErr("task-context: "+err.Error(), "")
			}
			targetContext, err := readOptionalContextArg(targetContextArg)
			if err != nil {
				return EmitErr("target-context: "+err.Error(), "")
			}
			targetContextInfo := targetContextCollectionInfo{Mode: "explicit", Bytes: len(targetContext)}
			if targetContext == "" {
				targetContext, targetContextInfo = collectTargetContextWithOptions(url, targetContextOptions{
					Mode:     targetContextMode,
					MaxBytes: maxTargetContextBytes,
				})
			}
			prMeta := collectPRMetadata(url)
			if rulesStatus == "" || rulesDigest == "" {
				st, stErr := computeProjectRulesStatus(currentProjectRoot())
				if stErr == nil {
					if rulesStatus == "" {
						rulesStatus = st.Status
					}
					if rulesDigest == "" {
						rulesDigest = st.RulesDigest
					}
				}
			}
			if rulesRead == "" {
				rulesRead = "yes"
			}
			req := codereview.Request{
				URL:            url,
				ProjectContext: projectContext,
				TaskContext:    taskContext,
				TargetContext:  targetContext,
				OutputDir:      outDir,
				BaseSHA:        prMeta.BaseSHA,
				HeadSHA:        prMeta.HeadSHA,
				PRNumber:       prMeta.PRNumber,
				ProjectRules: codereview.ProjectRulesEnvelope{
					Status: rulesStatus,
					Digest: rulesDigest,
					Read:   rulesRead,
				},
			}
			runner, err := codeReviewRunner(runnerName, inputDir)
			if err != nil {
				return EmitErr(err.Error(), "")
			}
			result, err := codereview.Execute(context.Background(), req, runner)
			if err != nil && targetContextArg == "" && targetContextInfo.Mode != "compact" && isContextOverflowError(err) {
				targetContext, targetContextInfo = collectTargetContextWithOptions(url, targetContextOptions{
					Mode:     "compact",
					MaxBytes: maxTargetContextBytes,
				})
				req.TargetContext = targetContext
				result, err = codereview.Execute(context.Background(), req, runner)
			}
			if err != nil {
				return EmitErr("code-review failed: "+err.Error(), "")
			}
			commentID := ""
			if post {
				body, err := os.ReadFile(result.ReviewMDPath)
				if err != nil {
					return EmitErr("read review.md: "+err.Error(), "")
				}
				commentURL, postedID, err := postStandaloneReview(url, string(body), result.Verdict)
				if err != nil {
					return EmitErr("post review: "+err.Error(), "")
				}
				result.CommentURL = commentURL
				commentID = postedID
				if err := writeCodeReviewResult(result); err != nil {
					return EmitErr(err.Error(), "")
				}
			}
			if err := persistStandaloneCodeReviewEvidence(c, result, req, commentID); err != nil {
				return EmitErr(err.Error(), "")
			}
			return EmitJSON(map[string]any{
				"ok":                  true,
				"verdict":             result.Verdict,
				"review_integrity":    result.Integrity.Status,
				"review_md":           result.ReviewMDPath,
				"review_result_json":  result.ResultJSONPath,
				"review_summary_json": result.ReviewSummaryPath,
				"comment_url":         result.CommentURL,
			})
		},
	}
	c.Flags().String("url", "", "PR, branch, commit, or review target URL")
	c.Flags().String("project-context", "", "project context text or path")
	c.Flags().String("task-context", "", "task context text or path")
	c.Flags().String("target-context", "", "target context text or path (optional; GitHub PR context is collected when possible)")
	c.Flags().String("target-context-mode", "auto", "target context collection: auto, full, compact")
	c.Flags().Int("max-target-context-bytes", 256*1024, "max auto-collected target context bytes before compact mode")
	c.Flags().String("out", "", "output directory for review artifacts")
	c.Flags().String("runner", "codex", "review runner: codex or files")
	c.Flags().String("input-dir", "", "input fixture directory for --runner files")
	c.Flags().Bool("post", false, "post review.md to the target PR when URL is a GitHub PR")
	c.Flags().String("project-rules-status", "", "PROJECT_RULES_STATUS override")
	c.Flags().String("project-rules-digest", "", "PROJECT_RULES_DIGEST override")
	c.Flags().String("project-rules-read", "yes", "PROJECT_RULES_READ override")
	return c
}

func codeReviewRunner(name, inputDir string) (codereview.Runner, error) {
	switch strings.TrimSpace(name) {
	case "", "codex":
		return codereview.CodexRunner{WorkDir: currentProjectRoot()}, nil
	case "files":
		return codereview.FilesRunner{InputDir: inputDir}, nil
	default:
		return nil, fmt.Errorf("unknown code-review runner %q", name)
	}
}

func readContextArg(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", fmt.Errorf("value or path is required")
	}
	return readOptionalContextArg(value)
}

func readOptionalContextArg(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", nil
	}
	if body, err := os.ReadFile(value); err == nil {
		return string(body), nil
	}
	return value, nil
}

func collectTargetContext(url string) string {
	ctx, _ := collectTargetContextWithOptions(url, targetContextOptions{Mode: "auto", MaxBytes: 256 * 1024})
	return ctx
}

type targetContextOptions struct {
	Mode     string
	MaxBytes int
}

type targetContextCollectionInfo struct {
	Mode  string
	Bytes int
}

func collectTargetContextWithOptions(url string, opts targetContextOptions) (string, targetContextCollectionInfo) {
	if strings.TrimSpace(url) == "" {
		return "", targetContextCollectionInfo{Mode: "empty"}
	}
	mode := strings.ToLower(strings.TrimSpace(opts.Mode))
	if mode == "" {
		mode = "auto"
	}
	if opts.MaxBytes <= 0 {
		opts.MaxBytes = 256 * 1024
	}
	out, _, err := codeReviewRun(".", "gh", "pr", "view", url, "--json", "title,body,url,baseRefOid,headRefOid,files")
	if err != nil {
		ctx := "Target URL: " + url
		return ctx, targetContextCollectionInfo{Mode: "url", Bytes: len(ctx)}
	}
	diff, _, diffErr := codeReviewRun(".", "gh", "pr", "diff", url)
	if diffErr != nil {
		ctx := compactTargetContext(url, out, "", true)
		return ctx, targetContextCollectionInfo{Mode: "compact", Bytes: len(ctx)}
	}
	full := out + "\n\nDiff:\n" + diff
	if mode == "full" || (mode == "auto" && len(full) <= opts.MaxBytes) {
		return full, targetContextCollectionInfo{Mode: "full", Bytes: len(full)}
	}
	ctx := compactTargetContext(url, out, diff, false)
	return ctx, targetContextCollectionInfo{Mode: "compact", Bytes: len(ctx)}
}

func compactTargetContext(url, metadata, diff string, diffUnavailable bool) string {
	var parsed struct {
		Title      string `json:"title"`
		URL        string `json:"url"`
		BaseRefOID string `json:"baseRefOid"`
		HeadRefOID string `json:"headRefOid"`
		Files      []struct {
			Path      string `json:"path"`
			Additions int    `json:"additions"`
			Deletions int    `json:"deletions"`
		} `json:"files"`
	}
	_ = json.Unmarshal([]byte(metadata), &parsed)
	var b strings.Builder
	fmt.Fprintln(&b, "Context mode: compact")
	fmt.Fprintf(&b, "Target URL: %s\n", firstNonEmpty(parsed.URL, url))
	if parsed.Title != "" {
		fmt.Fprintf(&b, "Title: %s\n", parsed.Title)
	}
	if parsed.BaseRefOID != "" || parsed.HeadRefOID != "" {
		fmt.Fprintf(&b, "Base SHA: %s\nHead SHA: %s\n", parsed.BaseRefOID, parsed.HeadRefOID)
	}
	if len(parsed.Files) > 0 {
		fmt.Fprintln(&b, "Changed files:")
		for _, file := range parsed.Files {
			fmt.Fprintf(&b, "- %s (+%d/-%d)\n", file.Path, file.Additions, file.Deletions)
		}
	}
	fmt.Fprintln(&b)
	if diffUnavailable {
		fmt.Fprintln(&b, "Diff unavailable from GitHub CLI; review must fetch targeted patches if needed.")
	} else {
		fmt.Fprintf(&b, "Diff omitted from prompt because collected diff is %d bytes. Review must inspect targeted files/patches before approving.\n", len(diff))
	}
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "PR metadata JSON:")
	fmt.Fprintln(&b, metadata)
	return b.String()
}

func isContextOverflowError(err error) bool {
	if err == nil {
		return false
	}
	lower := strings.ToLower(err.Error())
	for _, needle := range []string{"context window", "maximum context", "context length", "ran out of room", "too large"} {
		if strings.Contains(lower, needle) {
			return true
		}
	}
	return false
}

type prMetadata struct {
	PRNumber string
	BaseSHA  string
	HeadSHA  string
}

func collectPRMetadata(url string) prMetadata {
	var meta prMetadata
	_, number, err := parseGitHubPRURL(url)
	if err == nil {
		meta.PRNumber = number
	}
	out, _, err := codeReviewRun(".", "gh", "pr", "view", url, "--json", "number,baseRefOid,headRefOid")
	if err != nil {
		return meta
	}
	var parsed struct {
		Number     int    `json:"number"`
		BaseRefOID string `json:"baseRefOid"`
		HeadRefOID string `json:"headRefOid"`
	}
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		return meta
	}
	if parsed.Number > 0 {
		meta.PRNumber = fmt.Sprintf("%d", parsed.Number)
	}
	meta.BaseSHA = parsed.BaseRefOID
	meta.HeadSHA = parsed.HeadRefOID
	return meta
}

var codeReviewRun = run

func postStandaloneReview(url, body, verdict string) (string, string, error) {
	repo, number, err := parseGitHubPRURL(url)
	if err != nil {
		return "", "", err
	}
	if verdict == codereview.VerdictChangesRequested {
		if commentURL, postedID, err := postPullRequestReview(repo, number, body); err == nil {
			return commentURL, postedID, nil
		}
	}
	return postIssueComment(repo, number, body)
}

func postPullRequestReview(repo, number, body string) (string, string, error) {
	input, err := writeTempJSON("codedungeon-code-review-*.json", map[string]string{
		"body":  body,
		"event": "REQUEST_CHANGES",
	})
	if err != nil {
		return "", "", err
	}
	defer os.Remove(input)
	out, errb, err := codeReviewRun(".", "gh", "api", "-X", "POST", fmt.Sprintf("/repos/%s/pulls/%s/reviews", repo, number), "--input", input)
	if err != nil {
		return "", "", fmt.Errorf("%s", strings.TrimSpace(errb))
	}
	return parsePostedReview(out)
}

func postIssueComment(repo, number, body string) (string, string, error) {
	input, err := writeTempJSON("codedungeon-code-review-*.json", map[string]string{"body": body})
	if err != nil {
		return "", "", err
	}
	defer os.Remove(input)
	out, errb, err := codeReviewRun(".", "gh", "api", "-X", "POST", fmt.Sprintf("/repos/%s/issues/%s/comments", repo, number), "--input", input)
	if err != nil {
		return "", "", fmt.Errorf("%s", strings.TrimSpace(errb))
	}
	return parsePostedReview(out)
}

func writeTempJSON(pattern string, payload any) (string, error) {
	input, err := os.CreateTemp("", pattern)
	if err != nil {
		return "", err
	}
	defer input.Close()
	body, err := json.Marshal(payload)
	if err != nil {
		_ = os.Remove(input.Name())
		return "", err
	}
	if _, err := input.Write(body); err != nil {
		_ = os.Remove(input.Name())
		return "", err
	}
	return input.Name(), nil
}

func parsePostedReview(out string) (string, string, error) {
	var posted struct {
		ID      int64  `json:"id"`
		HTMLURL string `json:"html_url"`
	}
	if err := json.Unmarshal([]byte(out), &posted); err != nil {
		return "", "", err
	}
	return posted.HTMLURL, fmt.Sprintf("%d", posted.ID), nil
}

func parseGitHubPRURL(url string) (string, string, error) {
	re := regexp.MustCompile(`github\.com/([^/]+/[^/]+)/pull/([0-9]+)`)
	match := re.FindStringSubmatch(url)
	if len(match) != 3 {
		return "", "", fmt.Errorf("url is not a GitHub pull request URL")
	}
	return match[1], match[2], nil
}

func writeCodeReviewResult(result codereview.Result) error {
	for _, path := range []string{result.ResultJSONPath, result.ReviewJSONPath} {
		body, err := json.MarshalIndent(result, "", "  ")
		if err != nil {
			return err
		}
		body = append(body, '\n')
		if err := os.WriteFile(path, body, 0o644); err != nil {
			return err
		}
	}
	if result.ReviewSummaryPath != "" {
		body, err := json.MarshalIndent(result.Summary, "", "  ")
		if err != nil {
			return err
		}
		body = append(body, '\n')
		if err := os.WriteFile(result.ReviewSummaryPath, body, 0o644); err != nil {
			return err
		}
	}
	return nil
}

func persistStandaloneCodeReviewEvidence(c *cobra.Command, result codereview.Result, req codereview.Request, commentID string) error {
	dbPath := ""
	if c != nil && c.Root() != nil {
		if flag := c.Root().PersistentFlags().Lookup("db"); flag != nil {
			dbPath = flag.Value.String()
		}
	}
	if dbPath == "" {
		dbPath = projectPath(currentProjectRoot(), provider.Detect().DBPath())
	}
	if _, err := os.Stat(dbPath); err != nil {
		return nil
	}
	s, err := OpenDB(c)
	if err != nil {
		if errors.Is(err, ErrNoGit) || errors.Is(err, ErrNoProject) || errors.Is(err, ErrHomeConfig) {
			return nil
		}
		return err
	}
	defer s.Close()
	if err := s.Init(); err != nil {
		return err
	}
	runRow, err := s.CurrentRun()
	if err != nil || runRow == nil {
		return err
	}
	reviewID, err := s.InsertReviewEvidence(db.ReviewEvidence{
		RunID:            runRow.ID,
		ReviewDir:        req.OutputDir,
		ReviewJSONPath:   result.ReviewJSONPath,
		ManifestPath:     filepath.Join(req.OutputDir, "review-request.json"),
		Verdict:          result.Verdict,
		PRNumber:         req.PRNumber,
		BaseSHA:          req.BaseSHA,
		HeadSHA:          req.HeadSHA,
		PersonasExpected: codereview.DefaultPersonas,
		PersonasRun:      codeReviewPersonaNames(result.Personas),
	})
	if err != nil {
		return err
	}
	registry := artifactreg.NewRegistry(s, currentProjectRoot())
	meta := map[string]any{"verdict": result.Verdict, "pr_number": req.PRNumber}
	for _, item := range []struct {
		role string
		kind string
		path string
	}{
		{"directory", "directory", req.OutputDir},
		{"request", "json", filepath.Join(req.OutputDir, "review-request.json")},
		{"result", "json", result.ResultJSONPath},
		{"review_json", "json", result.ReviewJSONPath},
		{"review_md", "markdown", result.ReviewMDPath},
		{"summary", "json", result.ReviewSummaryPath},
		{"decision", "json", result.DecisionJSONPath},
	} {
		if err := artifactreg.RegisterIfExists(registry, artifactreg.Record{
			RunID: runRow.ID, Module: "review", OwnerType: "review_evidence", OwnerID: strconv.FormatInt(reviewID, 10),
			Phase: "5.5", Role: item.role, Kind: item.kind, Path: item.path, Metadata: meta,
		}); err != nil {
			return err
		}
	}
	if _, err := s.InsertRunEvent(db.RunEvent{
		RunID:     runRow.ID,
		SessionID: os.Getenv(envSessionID),
		Event:     "code_review_integrity_pass",
		Detail:    result.ResultJSONPath,
	}); err != nil {
		return err
	}
	if result.CommentURL != "" && req.PRNumber != "" {
		body, err := os.ReadFile(result.ReviewMDPath)
		if err != nil {
			return err
		}
		sum := sha256.Sum256([]byte(strings.TrimSpace(string(body))))
		_, err = s.InsertPRReviewPost(db.PRReviewPost{
			RunID:            runRow.ID,
			ReviewEvidenceID: reviewID,
			PRNumber:         req.PRNumber,
			CommentID:        commentID,
			CommentURL:       result.CommentURL,
			BodySHA256:       fmt.Sprintf("%x", sum[:]),
		})
		return err
	}
	return nil
}

func codeReviewPersonaNames(personas []codereview.PersonaReview) []string {
	out := make([]string, 0, len(personas))
	for _, persona := range personas {
		out = append(out, persona.Persona)
	}
	return out
}
