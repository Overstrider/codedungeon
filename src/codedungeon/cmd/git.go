package cmd

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/loldinis/codedungeon/internal/osadapter"
	"github.com/loldinis/codedungeon/internal/provider"
)

var protectedBranches = []string{"main", "master", "develop", "dev", "staging", "production", "release"}

func GitCmd() *cobra.Command {
	c := &cobra.Command{Use: "git", Short: "git + gh wrappers"}
	c.AddCommand(gitGuardCmd())
	c.AddCommand(gitPRCmd())
	c.AddCommand(gitVerifyCmd())
	c.AddCommand(gitDiffCmd())
	return c
}

// run delegates to the OS adapter. Resolves well-known tools (git / gh) via
// FindTool so Windows hints like C:\tools\gh\gh.exe are honored even when the
// tool is not on PATH.
func run(repo string, args ...string) (string, string, error) {
	ad := osadapter.Detect()
	tool := args[0]
	switch tool {
	case "git", "gh":
		if resolved, err := ad.FindTool(tool); err == nil {
			tool = resolved
		}
	}
	return ad.RunExec(repo, tool, args[1:]...)
}

func currentBranch(repo string) (string, error) {
	out, errb, err := run(repo, "git", "branch", "--show-current")
	if err != nil {
		return "", fmt.Errorf("git branch: %s (%w)", errb, err)
	}
	return out, nil
}

func gitGuardCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "guard",
		Short: "Fail if current branch is protected (main/master/develop/...)",
		RunE: func(c *cobra.Command, _ []string) error {
			repo, _ := c.Flags().GetString("repo")
			br, err := currentBranch(repo)
			if err != nil {
				return EmitErr(err.Error(), "")
			}
			for _, p := range protectedBranches {
				if br == p {
					return EmitErr("on protected branch: "+br, "switch to a feature branch before committing")
				}
			}
			return EmitJSON(map[string]any{"ok": true, "branch": br, "protected": false})
		},
	}
	c.Flags().String("repo", ".", "repo dir")
	return c
}

func gitPRCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "pr",
		Short: "Return PR number (and optional context) for the current branch",
		RunE: func(c *cobra.Command, _ []string) error {
			repo, _ := c.Flags().GetString("repo")
			branch, _ := c.Flags().GetString("branch")
			withCtx, _ := c.Flags().GetBool("with-context")
			if branch == "" {
				b, err := currentBranch(repo)
				if err != nil {
					return EmitErr(err.Error(), "")
				}
				branch = b
			}
			out, errb, err := run(repo, "gh", "pr", "list", "--head", branch, "--json", "number,title,body,url", "-q", ".[0]")
			if err != nil {
				return EmitErr("gh pr list failed: "+errb, "")
			}
			if out == "" || out == "null" {
				return EmitJSON(map[string]any{"ok": true, "branch": branch, "pr": nil})
			}
			result := map[string]any{"ok": true, "branch": branch, "pr_raw": out}
			if withCtx {
				issue, _, _ := run(repo, "gh", "pr", "view", "--json", "body", "-q", ".body")
				result["issue_body"] = issue
			}
			return EmitJSON(result)
		},
	}
	c.Flags().String("repo", ".", "repo dir")
	c.Flags().String("branch", "", "branch (default: current)")
	c.Flags().Bool("with-context", false, "also fetch body + linked issue")
	return c
}

func gitVerifyCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "verify",
		Short: "Verify: not on protected branch, no unpushed commits, PR exists, adversarial review posted",
		RunE: func(c *cobra.Command, _ []string) error {
			repo, _ := c.Flags().GetString("repo")
			branch, _ := c.Flags().GetString("branch")
			status, err := gitVerifyStatus(repo, branch)
			if err != nil {
				return EmitErr(err.Error(), "")
			}
			return EmitJSON(status)
		},
	}
	c.Flags().String("repo", ".", "repo dir")
	c.Flags().String("branch", "", "branch (default: current)")
	return c
}

func gitVerifyStatus(repo, branch string) (map[string]any, error) {
	if branch == "" {
		b, err := currentBranch(repo)
		if err != nil {
			return nil, err
		}
		branch = b
	}
	for _, p := range protectedBranches {
		if branch == p {
			return nil, fmt.Errorf("on protected branch: %s", branch)
		}
	}
	unpushed, _, _ := run(repo, "git", "log", fmt.Sprintf("origin/%s..%s", branch, branch), "--oneline")
	prNum, _, _ := run(repo, "gh", "pr", "list", "--head", branch, "--json", "number", "-q", ".[0].number")
	prOpen := false
	if prNum != "" {
		state, _, _ := run(repo, "gh", "pr", "view", prNum, "--json", "state", "-q", ".state")
		prOpen = strings.TrimSpace(state) == "OPEN"
	}
	reviewOK, reviewDetail := recordedReviewCommentOK(repo, prNum)
	pass := prNum != "" && prOpen && reviewOK && strings.TrimSpace(unpushed) == ""
	return map[string]any{
		"ok":                      pass,
		"branch":                  branch,
		"protected":               false,
		"unpushed_commits":        strings.Count(unpushed, "\n") + boolToInt(unpushed != ""),
		"pr_number":               prNum,
		"pr_open":                 prOpen,
		"recorded_review_comment": reviewOK,
		"review_comment_detail":   reviewDetail,
		"legacy_marker_ignored":   provider.Detect().ReviewCommentMarker(),
	}, nil
}

func recordedReviewCommentOK(repo, prNum string) (bool, string) {
	if prNum == "" {
		return false, "missing PR"
	}
	s, err := OpenDB(&cobra.Command{})
	if err != nil {
		return false, err.Error()
	}
	defer s.Close()
	runRow, err := s.CurrentRun()
	if err != nil || runRow == nil {
		return false, "missing active run"
	}
	post, err := s.LatestPRReviewPost(runRow.ID)
	if err != nil {
		return false, err.Error()
	}
	if post == nil {
		return false, "missing codedungeon review post evidence"
	}
	if post.PRNumber != prNum {
		return false, "review post PR mismatch"
	}
	repoName, errb, err := run(repo, "gh", "repo", "view", "--json", "nameWithOwner", "-q", ".nameWithOwner")
	if err != nil {
		return false, "gh repo view failed: " + errb
	}
	body, errb, err := run(repo, "gh", "api", fmt.Sprintf("/repos/%s/issues/comments/%s", strings.TrimSpace(repoName), post.CommentID), "--jq", ".body")
	if err != nil {
		return false, "gh api comment failed: " + errb
	}
	sum := sha256.Sum256([]byte(strings.TrimSpace(body)))
	if hex.EncodeToString(sum[:]) != post.BodySHA256 {
		return false, "review comment body hash mismatch"
	}
	if !strings.Contains(body, "CodeDungeon Code Review") {
		return false, "review comment missing standalone code-review marker"
	}
	return true, post.CommentURL
}

func gitDiffCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "diff",
		Short: "git diff wrapper",
		RunE: func(c *cobra.Command, _ []string) error {
			repo, _ := c.Flags().GetString("repo")
			base, _ := c.Flags().GetString("base")
			mode, _ := c.Flags().GetString("mode")
			spec := fmt.Sprintf("%s...HEAD", base)
			var args []string
			switch mode {
			case "changed-files":
				args = []string{"git", "diff", spec, "--name-only"}
			case "stat":
				args = []string{"git", "diff", spec, "--stat"}
			case "full", "":
				args = []string{"git", "diff", spec}
			default:
				return EmitErr("invalid --mode", "one of: changed-files, stat, full")
			}
			out, errb, err := run(repo, args...)
			if err != nil {
				return EmitErr("git diff failed: "+errb, "")
			}
			return EmitJSON(map[string]any{"ok": true, "mode": mode, "content": out})
		},
	}
	c.Flags().String("repo", ".", "repo dir")
	c.Flags().String("base", "main", "base ref")
	c.Flags().String("mode", "full", "full | changed-files | stat")
	return c
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
