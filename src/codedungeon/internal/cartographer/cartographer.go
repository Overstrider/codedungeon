package cartographer

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type Options struct {
	MaxTokens int
}

type Result struct {
	Root        string        `json:"root"`
	Files       []File        `json:"files"`
	Directories []string      `json:"directories"`
	TotalTokens int           `json:"total_tokens"`
	TotalFiles  int           `json:"total_files"`
	Skipped     []SkippedFile `json:"skipped"`
}

type File struct {
	Path      string `json:"path"`
	Tokens    int    `json:"tokens"`
	SizeBytes int64  `json:"size_bytes"`
}

type SkippedFile struct {
	Path      string `json:"path"`
	Reason    string `json:"reason"`
	SizeBytes int64  `json:"size_bytes,omitempty"`
	Tokens    int    `json:"tokens,omitempty"`
}

func Scan(root string, opts Options) (Result, error) {
	abs, err := filepath.Abs(root)
	if err != nil {
		return Result{}, err
	}
	if opts.MaxTokens <= 0 {
		opts.MaxTokens = 50000
	}
	gitignore := parseGitignore(abs)
	result := Result{Root: abs}
	err = filepath.WalkDir(abs, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(abs, path)
		rel = filepath.ToSlash(rel)
		if rel == "." {
			return nil
		}
		name := d.Name()
		if shouldIgnore(rel, name, d.IsDir(), gitignore) {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if d.IsDir() {
			result.Directories = append(result.Directories, rel)
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		if !isText(name) {
			result.Skipped = append(result.Skipped, SkippedFile{Path: rel, Reason: "binary-or-unsupported", SizeBytes: info.Size()})
			return nil
		}
		tokens := int(info.Size() / 4)
		if info.Size() > 0 && tokens == 0 {
			tokens = 1
		}
		if tokens > opts.MaxTokens {
			result.Skipped = append(result.Skipped, SkippedFile{Path: rel, Reason: "too-large", SizeBytes: info.Size(), Tokens: tokens})
			return nil
		}
		result.Files = append(result.Files, File{Path: rel, Tokens: tokens, SizeBytes: info.Size()})
		result.TotalTokens += tokens
		result.TotalFiles++
		return nil
	})
	if err != nil {
		return Result{}, err
	}
	sort.Slice(result.Files, func(i, j int) bool { return result.Files[i].Path < result.Files[j].Path })
	sort.Strings(result.Directories)
	sort.Slice(result.Skipped, func(i, j int) bool { return result.Skipped[i].Path < result.Skipped[j].Path })
	return result, nil
}

func RenderCompact(result Result) string {
	files := append([]File(nil), result.Files...)
	sort.Slice(files, func(i, j int) bool {
		if files[i].Tokens == files[j].Tokens {
			return files[i].Path < files[j].Path
		}
		return files[i].Tokens > files[j].Tokens
	})
	var b strings.Builder
	for _, f := range files {
		fmt.Fprintf(&b, "%6d %s\n", f.Tokens, f.Path)
	}
	return b.String()
}

func RenderTree(result Result) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s\n", result.Root)
	for _, d := range result.Directories {
		fmt.Fprintf(&b, "  %s/\n", d)
	}
	for _, f := range result.Files {
		fmt.Fprintf(&b, "  %s (%d tokens)\n", f.Path, f.Tokens)
	}
	return b.String()
}

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

var textExts = map[string]bool{
	".py": true, ".js": true, ".ts": true, ".jsx": true, ".tsx": true,
	".vue": true, ".svelte": true, ".html": true, ".htm": true,
	".css": true, ".scss": true, ".sass": true, ".less": true,
	".json": true, ".yaml": true, ".yml": true, ".toml": true, ".xml": true,
	".md": true, ".mdx": true, ".txt": true, ".rst": true,
	".sh": true, ".bash": true, ".zsh": true, ".fish": true,
	".ps1": true, ".bat": true, ".cmd": true, ".sql": true,
	".graphql": true, ".gql": true, ".proto": true,
	".go": true, ".rs": true, ".rb": true, ".php": true,
	".java": true, ".kt": true, ".kts": true, ".scala": true,
	".clj": true, ".cljs": true, ".edn": true, ".ex": true, ".exs": true,
	".erl": true, ".hrl": true, ".hs": true, ".lhs": true, ".ml": true,
	".mli": true, ".fs": true, ".fsx": true, ".fsi": true, ".cs": true,
	".vb": true, ".swift": true, ".m": true, ".mm": true, ".h": true,
	".hpp": true, ".hh": true, ".hxx": true, ".c": true, ".cc": true,
	".cpp": true, ".cxx": true, ".dart": true, ".lua": true, ".pl": true,
	".pm": true, ".r": true, ".gradle": true, ".tf": true, ".tfvars": true,
	".dockerfile": true, ".dockerignore": true, ".gitignore": true,
	".env": true, ".editorconfig": true, ".mod": true, ".sum": true,
	".work": true, ".cfg": true, ".ini": true, ".conf": true,
	".properties": true, ".csv": true, ".tsv": true, ".log": true,
}

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
			continue
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
			if ok, _ := filepath.Match(p, rel); ok {
				return true
			}
			if ok, _ := filepath.Match(p+"/**", rel); ok {
				return true
			}
		} else if ok, _ := filepath.Match(p, name); ok {
			return true
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
	return matchesGitignore(relPath, name, isDir, gitignore)
}

func isText(name string) bool {
	ext := strings.ToLower(filepath.Ext(name))
	if textExts[ext] {
		return true
	}
	return ext == "" && noExtTextNames[name]
}
