package cmd

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestReleaseBuildScriptBuildsProviderSpecificBinaries(t *testing.T) {
	body, err := os.ReadFile(filepath.Join(repoRoot(t), "scripts", "build-release.ps1"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(body)
	for _, want := range []string{
		"codedungeon-codex",
		"codedungeon-claude",
		"Provider = 'codex'",
		"Provider = 'claude'",
		"-X github.com/loldinis/codedungeon/internal/provider.DefaultProvider=$($Target.Provider)",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("scripts/build-release.ps1 missing %q", want)
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
			".codedungeon/bin",
			"setup --target",
			"--is-inside-work-tree",
		},
		"release/install.ps1": {
			"codedungeon-$Provider",
			".codedungeon/bin",
			"--is-inside-work-tree",
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
		if strings.HasPrefix(file, "release/install.") && strings.Contains(text, "--show-toplevel") {
			t.Fatalf("%s must not use git show-toplevel to choose install target", file)
		}
	}
}

func TestReleaseV2MetadataAndGuidance(t *testing.T) {
	root := repoRoot(t)
	files := map[string][]string{
		"scripts/build-release.ps1": {
			`[string]$Version = 'v2.0.0'`,
			"codedungeon-codex.exe",
			"codedungeon-claude.exe",
		},
		"release/install.sh": {
			".codedungeon/bin",
			"setup --target",
			"--is-inside-work-tree",
		},
		"release/install.ps1": {
			".codedungeon/bin",
			"setup --target",
			"--is-inside-work-tree",
		},
		"README.md": {
			"Repository maintenance is main-only",
			"installed CodeDungeon workflows remain PR-centered",
			"Project Rules discovery",
		},
		"release/README.md": {
			"v2.0.0",
			"Run Project Rules discovery before the first real task",
		},
		"release/QUICKSTART.md": {
			"v2.0.0",
			"Run Project Rules discovery before the first real task",
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
		if _, err := os.Stat(filepath.Join(dir, "scripts", "build-release.ps1")); err == nil {
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
