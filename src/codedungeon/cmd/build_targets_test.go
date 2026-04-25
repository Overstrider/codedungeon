package cmd

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestMakefileBuildsProviderSpecificBinaries(t *testing.T) {
	body, err := os.ReadFile(filepath.Join(repoRoot(t), "Makefile"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(body)
	for _, want := range []string{
		"codedungeon-codex",
		"codedungeon-claude",
		"-X github.com/loldinis/codedungeon/internal/provider.DefaultProvider=codex",
		"-X github.com/loldinis/codedungeon/internal/provider.DefaultProvider=claude",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("Makefile missing %q", want)
		}
	}
}

func TestInstallersAndDocsExposeProviderChoice(t *testing.T) {
	root := repoRoot(t)
	files := map[string][]string{
		"install.sh": {
			"release/install.sh",
			"--provider codex",
			"--provider claude",
		},
		"install.ps1": {
			"release/install.ps1",
			"ValidateSet('codex','claude','claude-code','claude-ce')",
		},
		"release/install.sh": {
			"claude-code|claude-ce",
			"PLUGIN_BIN=\"$PLUG/bin/codedungeon\"",
		},
		"release/install.ps1": {
			"codedungeon-$Provider",
			"codedungeon.exe",
		},
		"README.md": {
			"codedungeon-codex",
			"codedungeon-claude",
			"Codex CLI",
			"Claude Code",
		},
	}
	for file, wants := range files {
		body, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(file)))
		if err != nil {
			t.Fatalf("read %s: %v", file, err)
		}
		text := string(body)
		for _, want := range wants {
			if !strings.Contains(text, want) {
				t.Fatalf("%s missing %q", file, want)
			}
		}
	}
}

func repoRoot(t *testing.T) string {
	t.Helper()
	if cwd, err := os.Getwd(); err == nil {
		if root, ok := findRepoRoot(cwd); ok {
			return root
		}
	}
	if _, file, _, ok := runtime.Caller(0); ok {
		if root, ok := findRepoRoot(filepath.Dir(file)); ok {
			return root
		}
	}
	t.Fatal("cannot resolve repo root")
	return ""
}

func findRepoRoot(start string) (string, bool) {
	dir, _ := filepath.Abs(start)
	for i := 0; i < 8; i++ {
		if _, err := os.Stat(filepath.Join(dir, "Makefile")); err == nil {
			if _, err := os.Stat(filepath.Join(dir, "src", "codedungeon", "go.mod")); err == nil {
				return dir, true
			}
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return "", false
}
