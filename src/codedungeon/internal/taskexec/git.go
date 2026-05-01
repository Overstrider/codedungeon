package taskexec

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
)

type ShellGit struct {
	Policy SafetyPolicy
}

func (g ShellGit) Head(ctx context.Context, repo string) (string, error) {
	out, err := g.run(ctx, repo, "rev-parse", "HEAD")
	return strings.TrimSpace(out), err
}

func (g ShellGit) CreateBackupRef(ctx context.Context, repo, name, head string) error {
	if head == "" {
		return nil
	}
	_, err := g.run(ctx, repo, "branch", name, head)
	return err
}

func (g ShellGit) ChangedFiles(ctx context.Context, repo string) ([]string, error) {
	out, err := g.run(ctx, repo, "diff", "--name-only", "HEAD")
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

func (g ShellGit) Diff(ctx context.Context, repo string) (string, error) {
	return g.run(ctx, repo, "diff")
}

func (g ShellGit) Commit(ctx context.Context, repo, message string) error {
	if _, err := g.run(ctx, repo, "add", "-A"); err != nil {
		return err
	}
	out, err := g.run(ctx, repo, "diff", "--cached", "--quiet")
	if err == nil && strings.TrimSpace(out) == "" {
		return nil
	}
	_, err = g.run(ctx, repo, "commit", "-m", message)
	return err
}

func (g ShellGit) Push(ctx context.Context, repo string) error {
	_, err := g.run(ctx, repo, "push")
	return err
}

func (g ShellGit) LatestSemverTag(ctx context.Context, repo string) (string, error) {
	if err := g.Policy.ValidateGit("tag"); err != nil {
		return "", err
	}
	cmd := exec.CommandContext(ctx, "git", "tag", "--list", "v[0-9]*.[0-9]*.[0-9]*", "--sort=-v:refname")
	cmd.Dir = repo
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("git tag list failed: %w: %s", err, strings.TrimSpace(stderr.String()))
	}
	for _, line := range strings.Split(stdout.String(), "\n") {
		line = strings.TrimSpace(line)
		if semverRE.MatchString(line) {
			return line, nil
		}
	}
	return "", nil
}

func (g ShellGit) Tag(ctx context.Context, repo, tag, message string) error {
	_, err := g.run(ctx, repo, "tag", "-a", tag, "-m", message)
	return err
}

func (g ShellGit) run(ctx context.Context, repo, subcommand string, args ...string) (string, error) {
	if err := g.Policy.ValidateGit(subcommand); err != nil {
		return "", err
	}
	cmdArgs := append([]string{subcommand}, args...)
	cmd := exec.CommandContext(ctx, "git", cmdArgs...)
	cmd.Dir = repo
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	if err != nil {
		return stdout.String(), fmt.Errorf("git %s failed: %w: %s", subcommand, err, strings.TrimSpace(stderr.String()))
	}
	return stdout.String(), nil
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
