package tooladapter

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

type GitClient interface {
	CurrentBranch(ctx context.Context, repo string) (string, error)
	RemoteOrigin(ctx context.Context, repo string) (string, error)
	RevParse(ctx context.Context, repo string, args ...string) (string, error)
	Log(ctx context.Context, repo string, args ...string) (string, error)
	Diff(ctx context.Context, repo string, args ...string) (string, error)
	ChangedFiles(ctx context.Context, repo string) ([]string, error)
	AddAll(ctx context.Context, repo string) error
	Commit(ctx context.Context, repo, message string) error
	Push(ctx context.Context, repo string) error
	LatestSemverTag(ctx context.Context, repo string) (string, error)
	Tag(ctx context.Context, repo, tag, message string) error
	Run(ctx context.Context, repo string, args ...string) (string, string, error)
}

type ShellGitClient struct {
	Runner CommandRunner
}

func NewGitClient(runner CommandRunner) GitClient {
	if runner == nil {
		runner = NewSystemRunner()
	}
	return ShellGitClient{Runner: runner}
}

func (g ShellGitClient) CurrentBranch(ctx context.Context, repo string) (string, error) {
	out, _, err := g.Run(ctx, repo, "branch", "--show-current")
	return strings.TrimSpace(out), err
}

func (g ShellGitClient) RemoteOrigin(ctx context.Context, repo string) (string, error) {
	out, _, err := g.Run(ctx, repo, "remote", "get-url", "origin")
	return strings.TrimSpace(out), err
}

func (g ShellGitClient) RevParse(ctx context.Context, repo string, args ...string) (string, error) {
	out, _, err := g.Run(ctx, repo, append([]string{"rev-parse"}, args...)...)
	return strings.TrimSpace(out), err
}

func (g ShellGitClient) Log(ctx context.Context, repo string, args ...string) (string, error) {
	out, _, err := g.Run(ctx, repo, append([]string{"log"}, args...)...)
	return out, err
}

func (g ShellGitClient) Diff(ctx context.Context, repo string, args ...string) (string, error) {
	out, _, err := g.Run(ctx, repo, append([]string{"diff"}, args...)...)
	return out, err
}

func (g ShellGitClient) ChangedFiles(ctx context.Context, repo string) ([]string, error) {
	out, err := g.Diff(ctx, repo, "--name-only", "HEAD")
	if err != nil {
		return nil, err
	}
	var files []string
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			files = append(files, line)
		}
	}
	return files, nil
}

func (g ShellGitClient) AddAll(ctx context.Context, repo string) error {
	_, _, err := g.Run(ctx, repo, gitAddAllArgs()...)
	return err
}

func (g ShellGitClient) Commit(ctx context.Context, repo, message string) error {
	if err := g.AddAll(ctx, repo); err != nil {
		return err
	}
	_, _, quietErr := g.Run(ctx, repo, "diff", "--cached", "--quiet")
	if quietErr == nil {
		return nil
	}
	_, _, err := g.Run(ctx, repo, "commit", "-m", message)
	return err
}

func (g ShellGitClient) Push(ctx context.Context, repo string) error {
	_, _, err := g.Run(ctx, repo, "push")
	return err
}

func (g ShellGitClient) LatestSemverTag(ctx context.Context, repo string) (string, error) {
	out, _, err := g.Run(ctx, repo, "tag", "--list", "v[0-9]*.[0-9]*.[0-9]*", "--sort=-v:refname")
	if err != nil {
		return "", err
	}
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if semverRE.MatchString(line) {
			return line, nil
		}
	}
	return "", nil
}

func (g ShellGitClient) Tag(ctx context.Context, repo, tag, message string) error {
	_, _, err := g.Run(ctx, repo, "tag", "-a", tag, "-m", message)
	return err
}

func (g ShellGitClient) Run(ctx context.Context, repo string, args ...string) (string, string, error) {
	result, err := g.Runner.Run(ctx, Command{Dir: repo, Name: "git", Args: args})
	return result.Stdout, result.Stderr, err
}

func gitAddAllArgs() []string {
	return []string{
		"add", "-A", "--", ".",
		":(exclude).codedungeon/*.db",
		":(exclude).codedungeon/*.db-journal",
		":(exclude).codedungeon/logs/**",
	}
}

var semverRE = regexp.MustCompile(`^v([0-9]+)\.([0-9]+)\.([0-9]+)$`)

func NextPatchTag(latest string) string {
	m := semverRE.FindStringSubmatch(strings.TrimSpace(latest))
	if len(m) != 4 {
		return "v0.1.0"
	}
	major, _ := strconv.Atoi(m[1])
	minor, _ := strconv.Atoi(m[2])
	patch, _ := strconv.Atoi(m[3])
	return fmt.Sprintf("v%d.%d.%d", major, minor, patch+1)
}
