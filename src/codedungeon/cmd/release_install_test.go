package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReleaseInstallersAreProjectLocalOnly(t *testing.T) {
	repoRoot := filepath.Clean(filepath.Join("..", "..", ".."))
	for _, rel := range []string{
		filepath.Join("release", "install.sh"),
		filepath.Join("release", "install.ps1"),
	} {
		bodyBytes, err := os.ReadFile(filepath.Join(repoRoot, rel))
		if err != nil {
			t.Fatalf("read %s: %v", rel, err)
		}
		body := string(bodyBytes)
		for _, required := range []string{
			".codedungeon/bin",
			"setup",
		} {
			if !strings.Contains(body, required) {
				t.Fatalf("%s missing project-local installer contract %q:\n%s", rel, required, body)
			}
		}
		for _, forbidden := range []string{
			"$HOME/.local/bin",
			"$HOME/.claude",
			"$HOME/.claude/plugins",
			"$HOME/.claude/plugins/local/codedungeon",
			"$Plug/.claude-plugin",
			"Join-Path $HOME",
			".claude-plugin",
			"plugins/local/codedungeon",
			"~/.local/bin",
		} {
			if strings.Contains(body, forbidden) {
				t.Fatalf("%s contains global install reference %q:\n%s", rel, forbidden, body)
			}
		}
	}
}
