package tooladapter

import (
	"context"
	"strings"
)

type GitHubClient interface {
	AuthStatus(ctx context.Context, repo string) error
	RepoName(ctx context.Context, repo string) (string, error)
	PRList(ctx context.Context, repo string, args ...string) (string, error)
	PRView(ctx context.Context, repo string, args ...string) (string, error)
	PRDiff(ctx context.Context, repo string, args ...string) (string, error)
	API(ctx context.Context, repo string, args ...string) (string, string, error)
	Run(ctx context.Context, repo string, args ...string) (string, string, error)
}

type ShellGitHubClient struct {
	Runner CommandRunner
}

func NewGitHubClient(runner CommandRunner) GitHubClient {
	if runner == nil {
		runner = NewSystemRunner()
	}
	return ShellGitHubClient{Runner: runner}
}

func (g ShellGitHubClient) AuthStatus(ctx context.Context, repo string) error {
	_, _, err := g.Run(ctx, repo, "auth", "status")
	return err
}

func (g ShellGitHubClient) RepoName(ctx context.Context, repo string) (string, error) {
	out, _, err := g.Run(ctx, repo, "repo", "view", "--json", "nameWithOwner", "-q", ".nameWithOwner")
	return strings.TrimSpace(out), err
}

func (g ShellGitHubClient) PRList(ctx context.Context, repo string, args ...string) (string, error) {
	out, _, err := g.Run(ctx, repo, append([]string{"pr", "list"}, args...)...)
	return out, err
}

func (g ShellGitHubClient) PRView(ctx context.Context, repo string, args ...string) (string, error) {
	out, _, err := g.Run(ctx, repo, append([]string{"pr", "view"}, args...)...)
	return out, err
}

func (g ShellGitHubClient) PRDiff(ctx context.Context, repo string, args ...string) (string, error) {
	out, _, err := g.Run(ctx, repo, append([]string{"pr", "diff"}, args...)...)
	return out, err
}

func (g ShellGitHubClient) API(ctx context.Context, repo string, args ...string) (string, string, error) {
	return g.Run(ctx, repo, append([]string{"api"}, args...)...)
}

func (g ShellGitHubClient) Run(ctx context.Context, repo string, args ...string) (string, string, error) {
	result, err := g.Runner.Run(ctx, Command{Dir: repo, Name: "gh", Args: args})
	return result.Stdout, result.Stderr, err
}
