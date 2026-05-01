package qa

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/loldinis/codedungeon/internal/manifest"
)

func DetectFramework(path string) FrameworkResult {
	if strings.TrimSpace(path) == "" {
		path = "."
	}
	info, _ := manifest.Detect(path)
	fw, cfg, cmd := detectTestFramework(path, info.Lang)
	if info.Lang != "unknown" {
		return FrameworkResult{
			OK:          true,
			Path:        path,
			Lang:        info.Lang,
			Framework:   fw,
			Config:      cfg,
			RunCommand:  cmd,
			RunCommands: nonEmptyList(cmd),
		}
	}

	entries, err := os.ReadDir(path)
	if err != nil {
		return FrameworkResult{OK: true, Path: path, Lang: info.Lang, Framework: "unknown"}
	}
	var components []FrameworkComponent
	var runCommands []string
	var summary []string
	for _, entry := range entries {
		if !entry.IsDir() || shouldSkipFrameworkDir(entry.Name()) {
			continue
		}
		child := filepath.Join(path, entry.Name())
		childInfo, _ := manifest.Detect(child)
		if childInfo.Lang == "unknown" {
			continue
		}
		childFW, childCfg, childCmd := detectTestFramework(child, childInfo.Lang)
		displayCmd := ""
		if childCmd != "" {
			displayCmd = "cd " + entry.Name() + " && " + childCmd
			runCommands = append(runCommands, displayCmd)
			summary = append(summary, "("+displayCmd+")")
		}
		components = append(components, FrameworkComponent{
			Path:       entry.Name(),
			Lang:       childInfo.Lang,
			Framework:  childFW,
			Config:     childCfg,
			RunCommand: displayCmd,
		})
	}
	if len(components) > 1 {
		return FrameworkResult{
			OK:          true,
			Path:        path,
			Lang:        "multi",
			Framework:   "monorepo",
			RunCommand:  strings.Join(summary, " && "),
			Components:  components,
			RunCommands: runCommands,
		}
	}
	if len(components) == 1 {
		component := components[0]
		return FrameworkResult{
			OK:          true,
			Path:        path,
			Lang:        component.Lang,
			Framework:   component.Framework,
			Config:      component.Config,
			RunCommand:  component.RunCommand,
			Components:  components,
			RunCommands: runCommands,
		}
	}
	return FrameworkResult{OK: true, Path: path, Lang: "unknown", Framework: "unknown"}
}

func detectTestFramework(path, lang string) (string, string, string) {
	existsAt := func(p string) bool {
		_, err := os.Stat(filepath.Join(path, p))
		return err == nil
	}
	pkgJSON := func(key string) bool {
		body, err := os.ReadFile(filepath.Join(path, "package.json"))
		if err != nil {
			return false
		}
		return strings.Contains(string(body), `"`+key+`"`)
	}
	switch lang {
	case "python":
		if existsAt("pytest.ini") || pyprojectContains(path, "[tool.pytest") {
			return "pytest", "pytest.ini", "pytest"
		}
		return "unittest", "", "python -m unittest"
	case "nextjs", "react", "vue", "typescript":
		switch {
		case pkgJSON("@playwright/test") || pkgJSON("playwright"):
			return "playwright", detectFirstExisting(path, "playwright.config.ts", "playwright.config.js", "playwright.config.mjs"), "npx playwright test"
		case pkgJSON("vitest"):
			return "vitest", detectFirstExisting(path, "vitest.config.ts", "vitest.config.js"), "npx vitest run"
		case pkgJSON("jest"):
			return "jest", detectFirstExisting(path, "jest.config.ts", "jest.config.js", "jest.config.json"), "npx jest"
		case pkgJSON("mocha"):
			return "mocha", "", "npx mocha"
		}
		return "unknown", "", ""
	case "go":
		return "go test", "", "go test ./..."
	case "rust":
		return "cargo test", "Cargo.toml", "cargo test"
	case "kotlin":
		return "junit", "build.gradle.kts", "./gradlew test"
	case "elixir":
		return "exunit", "mix.exs", "mix test"
	case "cpp":
		return "ctest", "CMakeLists.txt", "ctest"
	case "php":
		return "phpunit", "phpunit.xml", "vendor/bin/phpunit"
	case "ruby":
		return "rspec", "Gemfile", "bundle exec rspec"
	default:
		return "unknown", "", ""
	}
}

func shouldSkipFrameworkDir(name string) bool {
	switch name {
	case ".git", ".codedungeon", ".codex", ".claude", ".agents", "node_modules", "target", "dist", "build", ".next", "coverage", ".cache":
		return true
	default:
		return false
	}
}

func pyprojectContains(path, needle string) bool {
	body, err := os.ReadFile(filepath.Join(path, "pyproject.toml"))
	if err != nil {
		return false
	}
	return strings.Contains(string(body), needle)
}

func detectFirstExisting(root string, names ...string) string {
	for _, name := range names {
		path := filepath.Join(root, name)
		if _, err := os.Stat(path); err == nil {
			return path
		}
	}
	return ""
}

func nonEmptyList(value string) []string {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	return []string{value}
}
