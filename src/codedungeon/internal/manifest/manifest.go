// Package manifest detects a project's primary language + framework
// by inspecting manifest files (Cargo.toml, package.json, go.mod, etc.).
// No third-party deps — pure stdlib + regex.
package manifest

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// Info is the result of a single-directory detect.
type Info struct {
	Lang      string `json:"lang"`      // rust|nextjs|react|vue|typescript|kotlin|go|python|elixir|cpp|php|ruby|unknown
	Framework string `json:"framework"` // actix|axum|next|react|express|chi|gin|fastapi|phoenix|laravel|... or ""
	Stack     string `json:"stack"`     // human summary e.g. "Go + Chi"
	HasSource bool   `json:"has_source"`
	Manifest  string `json:"manifest"` // filename of detected manifest
}

// SourceDirs are the conventional source dirs that mark "this is code, not docs".
var SourceDirs = []string{"src", "app", "lib", "cmd", "internal", "pkg", "main"}

// HasSourceDir returns true if any SourceDirs entry exists under root.
func HasSourceDir(root string) bool {
	for _, d := range SourceDirs {
		st, err := os.Stat(filepath.Join(root, d))
		if err == nil && st.IsDir() {
			return true
		}
	}
	return false
}

// Detect walks the root dir, finds a manifest, and returns Info.
// Returns Info{Lang:"unknown"} and nil error when nothing matches.
func Detect(root string) (Info, error) {
	// Check manifests in priority order (most specific first).
	type check struct {
		file   string
		detect func(string) (Info, error)
	}
	checks := []check{
		{"Cargo.toml", detectRust},
		{"package.json", detectPackageJSON},
		{"go.mod", detectGo},
		{"build.gradle.kts", detectKotlin},
		{"build.gradle", detectKotlin},
		{"pyproject.toml", detectPython},
		{"requirements.txt", detectPython},
		{"mix.exs", detectElixir},
		{"CMakeLists.txt", detectCpp},
		{"composer.json", detectPHP},
		{"Gemfile", detectRuby},
	}
	for _, c := range checks {
		p := filepath.Join(root, c.file)
		if _, err := os.Stat(p); err == nil {
			info, err := c.detect(p)
			if err == nil {
				info.Manifest = c.file
				// Manifest present → assume it's a code repo. Conventional src/
				// dirs are nice-to-have but not required (Kotlin Compose uses
				// composeApp/, monorepos use packages/*, etc.).
				info.HasSource = true
				if info.Stack == "" {
					info.Stack = stackSummary(info.Lang, info.Framework)
				}
				return info, nil
			}
		}
	}
	return Info{Lang: "unknown", HasSource: HasSourceDir(root)}, nil
}

func stackSummary(lang, framework string) string {
	if framework == "" {
		return capFirst(lang)
	}
	// Avoid "Nextjs + Next" redundancy when lang == framework-ish.
	l := strings.ToLower(lang)
	f := strings.ToLower(framework)
	if l == f || strings.HasPrefix(l, f) || strings.HasPrefix(f, l) {
		return capFirst(framework)
	}
	return capFirst(lang) + " + " + capFirst(framework)
}

func capFirst(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

// --- Rust ---

func detectRust(path string) (Info, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return Info{}, err
	}
	content := string(b)
	framework := firstMatch(content, []string{"actix-web", "actix", "axum", "poem", "rocket", "warp"})
	if framework == "actix-web" {
		framework = "actix"
	}
	return Info{Lang: "rust", Framework: framework}, nil
}

// --- package.json (Next.js / React / Vue / Express / plain TS) ---

type pkgJSON struct {
	Dependencies    map[string]string `json:"dependencies"`
	DevDependencies map[string]string `json:"devDependencies"`
}

func detectPackageJSON(path string) (Info, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return Info{}, err
	}
	var p pkgJSON
	if err := json.Unmarshal(b, &p); err != nil {
		// Fall back: still JS even if parse fails.
		return Info{Lang: "typescript"}, nil
	}
	hasDep := func(name string) bool {
		if _, ok := p.Dependencies[name]; ok {
			return true
		}
		_, ok := p.DevDependencies[name]
		return ok
	}
	switch {
	case hasDep("next"):
		return Info{Lang: "nextjs", Framework: "next"}, nil
	case hasDep("vue"):
		return Info{Lang: "vue", Framework: "vue"}, nil
	case hasDep("react"):
		return Info{Lang: "react", Framework: "react"}, nil
	case hasDep("express"):
		return Info{Lang: "typescript", Framework: "express"}, nil
	case hasDep("fastify"):
		return Info{Lang: "typescript", Framework: "fastify"}, nil
	case hasDep("nestjs") || hasDep("@nestjs/core"):
		return Info{Lang: "typescript", Framework: "nestjs"}, nil
	}
	return Info{Lang: "typescript"}, nil
}

// --- Go ---

var goModuleRE = regexp.MustCompile(`(?m)^module\s+([^\s]+)`)

func detectGo(path string) (Info, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return Info{}, err
	}
	content := string(b)
	framework := firstMatch(content, []string{"gin-gonic/gin", "labstack/echo", "go-chi/chi", "gofiber/fiber", "gorilla/mux", "grpc-go", "gqlgen"})
	// Simplify.
	switch {
	case strings.Contains(framework, "gin"):
		framework = "gin"
	case strings.Contains(framework, "echo"):
		framework = "echo"
	case strings.Contains(framework, "chi"):
		framework = "chi"
	case strings.Contains(framework, "fiber"):
		framework = "fiber"
	case strings.Contains(framework, "mux"):
		framework = "gorilla"
	}
	return Info{Lang: "go", Framework: framework}, nil
}

// --- Kotlin ---

func detectKotlin(path string) (Info, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return Info{}, err
	}
	content := string(b)
	framework := firstMatch(content, []string{"compose", "ktor", "spring"})
	return Info{Lang: "kotlin", Framework: framework}, nil
}

// --- Python ---

func detectPython(path string) (Info, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return Info{}, err
	}
	content := string(b)
	framework := firstMatch(content, []string{"fastapi", "django", "flask", "starlette", "aiohttp"})
	return Info{Lang: "python", Framework: framework}, nil
}

// --- Elixir ---

func detectElixir(path string) (Info, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return Info{}, err
	}
	content := string(b)
	framework := firstMatch(content, []string{"phoenix", "liveview"})
	return Info{Lang: "elixir", Framework: framework}, nil
}

// --- C++ ---

func detectCpp(path string) (Info, error) {
	return Info{Lang: "cpp"}, nil
}

// --- PHP ---

func detectPHP(path string) (Info, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return Info{}, err
	}
	content := string(b)
	framework := firstMatch(content, []string{"laravel", "symfony", "slim"})
	return Info{Lang: "php", Framework: framework}, nil
}

// --- Ruby ---

func detectRuby(path string) (Info, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return Info{}, err
	}
	content := string(b)
	framework := firstMatch(content, []string{"rails", "sinatra", "hanami"})
	return Info{Lang: "ruby", Framework: framework}, nil
}

// --- helpers ---

func firstMatch(haystack string, needles []string) string {
	hay := strings.ToLower(haystack)
	for _, n := range needles {
		if strings.Contains(hay, strings.ToLower(n)) {
			return n
		}
	}
	return ""
}
