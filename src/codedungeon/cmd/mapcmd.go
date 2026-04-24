package cmd

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/spf13/cobra"
)

// MapCmd (`codedungeon map <path>`) replaces the Python cartographer/scan-codebase.py.
// Walks a directory tree, respects .gitignore, outputs file paths + estimated tokens.
//
// Token estimate = len(bytes) / 4 (heuristic; Claude does not expose a pure-Go tokenizer).
// Output formats: json (default) | tree | compact.
func MapCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "map <path>",
		Short: "Scan codebase — file tree + estimated tokens (Go port of cartographer)",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			path := "."
			if len(args) > 0 {
				path = args[0]
			}
			abs, err := filepath.Abs(path)
			if err != nil {
				return EmitErr(err.Error(), "")
			}
			format, _ := c.Flags().GetString("format")
			maxTokens, _ := c.Flags().GetInt("max-tokens")

			result, err := scanDir(abs, maxTokens)
			if err != nil {
				return EmitErr(err.Error(), "")
			}
			switch format {
			case "tree":
				fmt.Print(renderTree(result))
				return nil
			case "compact":
				renderCompact(result)
				return nil
			default:
				enc := json.NewEncoder(os.Stdout)
				enc.SetIndent("", "  ")
				return enc.Encode(result)
			}
		},
	}
	c.Flags().String("format", "json", "json | tree | compact")
	c.Flags().Int("max-tokens", 50000, "skip files exceeding this token estimate")
	return c
}

// scanResult matches the Python schema so consumers can swap transparently.
type scanResult struct {
	Root        string        `json:"root"`
	Files       []scanFile    `json:"files"`
	Directories []string      `json:"directories"`
	TotalTokens int           `json:"total_tokens"`
	TotalFiles  int           `json:"total_files"`
	Skipped     []skippedFile `json:"skipped"`
}

type scanFile struct {
	Path      string `json:"path"`
	Tokens    int    `json:"tokens"`
	SizeBytes int64  `json:"size_bytes"`
}

type skippedFile struct {
	Path      string `json:"path"`
	Reason    string `json:"reason"`
	SizeBytes int64  `json:"size_bytes,omitempty"`
	Tokens    int    `json:"tokens,omitempty"`
}

// defaultIgnoreDirs are always excluded.
var defaultIgnoreDirs = map[string]bool{
	".git": true, ".svn": true, ".hg": true,
	"node_modules": true, "__pycache__": true, ".pytest_cache": true,
	".mypy_cache": true, ".ruff_cache": true,
	"venv": true, ".venv": true, "env": true, ".env": true,
	"dist": true, "build": true, ".next": true, ".nuxt": true, ".output": true,
	"coverage": true, ".coverage": true, ".nyc_output": true,
	"target": true, "vendor": true, ".bundle": true, ".cargo": true,
	".idea": true, ".vscode": true, ".DS_Store": true,
}

// defaultIgnoreGlob — file-name globs always excluded.
var defaultIgnoreGlob = []string{
	"*.pyc", "*.pyo", "*.so", "*.dylib", "*.dll", "*.exe",
	"*.o", "*.a", "*.lib", "*.class", "*.jar", "*.war", "*.egg", "*.whl",
	"*.lock", "package-lock.json", "yarn.lock", "pnpm-lock.yaml",
	"bun.lockb", "Cargo.lock", "poetry.lock", "Gemfile.lock", "composer.lock",
	"*.png", "*.jpg", "*.jpeg", "*.gif", "*.ico", "*.svg", "*.webp",
	"*.mp3", "*.mp4", "*.wav", "*.avi", "*.mov", "*.pdf",
	"*.zip", "*.tar", "*.gz", "*.rar", "*.7z",
	"*.woff", "*.woff2", "*.ttf", "*.eot", "*.otf",
	"*.min.js", "*.min.css", "*.map", "*.chunk.js", "*.bundle.js",
}

// textExts — known text-file extensions.
var textExts = map[string]bool{
	".py": true, ".js": true, ".ts": true, ".jsx": true, ".tsx": true,
	".vue": true, ".svelte": true, ".html": true, ".htm": true,
	".css": true, ".scss": true, ".sass": true, ".less": true,
	".json": true, ".yaml": true, ".yml": true, ".toml": true, ".xml": true,
	".md": true, ".mdx": true, ".txt": true, ".rst": true,
	".sh": true, ".bash": true, ".zsh": true, ".fish": true, ".ps1": true,
	".bat": true, ".cmd": true, ".sql": true, ".graphql": true, ".gql": true,
	".proto": true,
	".go": true, ".rs": true, ".rb": true, ".php": true,
	".java": true, ".kt": true, ".kts": true, ".scala": true,
	".clj": true, ".cljs": true, ".edn": true,
	".ex": true, ".exs": true, ".erl": true, ".hrl": true,
	".hs": true, ".lhs": true, ".ml": true, ".mli": true,
	".fs": true, ".fsx": true, ".fsi": true,
	".cs": true, ".vb": true, ".swift": true, ".m": true, ".mm": true,
	".h": true, ".hpp": true, ".hh": true, ".hxx": true,
	".c": true, ".cc": true, ".cpp": true, ".cxx": true,
	".dart": true, ".lua": true, ".pl": true, ".pm": true, ".r": true,
	".gradle": true, ".tf": true, ".tfvars": true,
	".dockerfile": true, ".dockerignore": true, ".gitignore": true,
	".env": true, ".editorconfig": true, "": true, // files without extension (e.g. Makefile)
}

// noExtTextNames — common files without extension that are text.
var noExtTextNames = map[string]bool{
	"Makefile": true, "Dockerfile": true, "Jenkinsfile": true,
	"Rakefile": true, "Gemfile": true, "Pipfile": true,
	"CHANGELOG": true, "LICENSE": true, "README": true, "AUTHORS": true,
}

func parseGitignore(root string) []string {
	var out []string
	f, err := os.Open(filepath.Join(root, ".gitignore"))
	if err != nil {
		return out
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		out = append(out, line)
	}
	return out
}

func matchesGitignore(rel, name string, isDir bool, patterns []string) bool {
	for _, p := range patterns {
		if strings.HasPrefix(p, "!") {
			continue // negation not supported
		}
		dirOnly := false
		if strings.HasSuffix(p, "/") {
			dirOnly = true
			p = strings.TrimSuffix(p, "/")
		}
		if dirOnly && !isDir {
			continue
		}
		p = strings.TrimPrefix(p, "/")
		if strings.Contains(p, "/") {
			// Path-ish pattern — match against rel.
			if ok, _ := filepath.Match(p, rel); ok {
				return true
			}
			if ok, _ := filepath.Match(p+"/**", rel); ok {
				return true
			}
		} else {
			if ok, _ := filepath.Match(p, name); ok {
				return true
			}
		}
	}
	return false
}

func shouldIgnore(relPath, name string, isDir bool, gitignore []string) bool {
	if defaultIgnoreDirs[name] {
		return true
	}
	for _, pat := range defaultIgnoreGlob {
		if strings.Contains(pat, "*") {
			if ok, _ := filepath.Match(pat, name); ok {
				return true
			}
		} else if name == pat {
			return true
		}
	}
	if matchesGitignore(relPath, name, isDir, gitignore) {
		return true
	}
	return false
}

func isText(name string) bool {
	ext := strings.ToLower(filepath.Ext(name))
	if textExts[ext] {
		return true
	}
	if ext == "" && noExtTextNames[name] {
		return true
	}
	return false
}

func estimateTokens(b []byte) int {
	// Heuristic: ~4 chars/token for mixed code+prose.
	// Not perfect, but no pure-Go Claude tokenizer exists.
	return len(b) / 4
}

func scanDir(root string, maxTokens int) (*scanResult, error) {
	st, err := os.Stat(root)
	if err != nil {
		return nil, err
	}
	if !st.IsDir() {
		return nil, fmt.Errorf("not a directory: %s", root)
	}
	result := &scanResult{
		Root:        root,
		Files:       []scanFile{},
		Directories: []string{},
		Skipped:     []skippedFile{},
	}
	gitignore := parseGitignore(root)

	err = filepath.Walk(root, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // swallow permission errors, continue
		}
		if p == root {
			return nil
		}
		rel, _ := filepath.Rel(root, p)
		rel = filepath.ToSlash(rel)
		name := info.Name()

		if shouldIgnore(rel, name, info.IsDir(), gitignore) {
			if info.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if info.IsDir() {
			result.Directories = append(result.Directories, rel)
			return nil
		}

		size := info.Size()
		if size > 1_000_000 {
			result.Skipped = append(result.Skipped, skippedFile{Path: rel, Reason: "too_large", SizeBytes: size})
			return nil
		}
		if !isText(name) {
			result.Skipped = append(result.Skipped, skippedFile{Path: rel, Reason: "binary"})
			return nil
		}
		body, err := os.ReadFile(p)
		if err != nil {
			result.Skipped = append(result.Skipped, skippedFile{Path: rel, Reason: "read_error:" + err.Error()})
			return nil
		}
		tokens := estimateTokens(body)
		if tokens > maxTokens {
			result.Skipped = append(result.Skipped, skippedFile{Path: rel, Reason: "too_many_tokens", Tokens: tokens})
			return nil
		}
		result.Files = append(result.Files, scanFile{Path: rel, Tokens: tokens, SizeBytes: size})
		result.TotalTokens += tokens
		return nil
	})
	if err != nil {
		return nil, err
	}
	result.TotalFiles = len(result.Files)
	sort.Slice(result.Files, func(i, j int) bool { return result.Files[i].Path < result.Files[j].Path })
	return result, nil
}

func renderTree(r *scanResult) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s/\n", filepath.Base(r.Root))
	fmt.Fprintf(&b, "Total: %d files, %d tokens\n\n", r.TotalFiles, r.TotalTokens)
	// Build nested tree.
	type node map[string]any
	tree := node{}
	for _, f := range r.Files {
		parts := strings.Split(f.Path, "/")
		cur := tree
		for _, p := range parts[:len(parts)-1] {
			sub, ok := cur[p].(node)
			if !ok {
				sub = node{}
				cur[p] = sub
			}
			cur = sub
		}
		cur[parts[len(parts)-1]] = f
	}
	var walk func(n node, prefix string)
	walk = func(n node, prefix string) {
		keys := make([]string, 0, len(n))
		for k := range n {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for i, k := range keys {
			isLast := i == len(keys)-1
			connector := "├── "
			ext := "│   "
			if isLast {
				connector = "└── "
				ext = "    "
			}
			switch v := n[k].(type) {
			case node:
				fmt.Fprintf(&b, "%s%s%s/\n", prefix, connector, k)
				walk(v, prefix+ext)
			case scanFile:
				fmt.Fprintf(&b, "%s%s%s (%d tokens)\n", prefix, connector, k, v.Tokens)
			}
		}
	}
	walk(tree, "")
	return b.String()
}

func renderCompact(r *scanResult) {
	files := make([]scanFile, len(r.Files))
	copy(files, r.Files)
	sort.Slice(files, func(i, j int) bool { return files[i].Tokens > files[j].Tokens })
	fmt.Printf("# %s\n# Total: %d files, %d tokens\n\n", r.Root, r.TotalFiles, r.TotalTokens)
	for _, f := range files {
		fmt.Printf("%8d %s\n", f.Tokens, f.Path)
	}
}
