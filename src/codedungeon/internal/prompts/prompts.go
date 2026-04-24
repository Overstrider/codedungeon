// Package prompts holds embedded prompt/template files and a helper to resolve
// them by logical name (without extension). Used as a fallback when the SQLite
// prompts table is absent or empty.
package prompts

import (
	"embed"
	"fmt"
	"io/fs"
	"path"
	"sort"
	"strings"
)

//go:embed all:files
var fsys embed.FS

// Get returns the contents of an embedded prompt by logical name.
// Extensions are tried in this order: .md, .tmpl, .json, then literal.
// `persona-schema-X` also resolves to `persona-schemas/X.json`.
func Get(name string) (string, error) {
	candidates := []string{
		"files/" + name + ".md",
		"files/" + name + ".tmpl",
		"files/" + name + ".json",
		"files/" + name,
	}
	if strings.HasPrefix(name, "persona-schema-") {
		candidates = append(candidates, "files/persona-schemas/"+strings.TrimPrefix(name, "persona-schema-")+".json")
	}
	for _, c := range candidates {
		b, err := fsys.ReadFile(c)
		if err == nil {
			return string(b), nil
		}
	}
	return "", fmt.Errorf("embedded prompt not found: %s", name)
}

// Artifact is one file from the embedded tree, ready to be written into
// <project>/.claude/<RelPath>.
type Artifact struct {
	RelPath string // e.g. "agents/gremlin-reviewer-saboteur.md"
	Content []byte
}

// Artifacts walks the embedded tree and returns every file under
// files/{agents,skills,commands,phases}/. Everything else (caveman, handoff,
// persona-schemas, templates at root) stays embedded-only (Get()-accessed).
func Artifacts() ([]Artifact, error) {
	var out []Artifact
	err := fs.WalkDir(fsys, "files", func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		rel := strings.TrimPrefix(p, "files/")
		if !hasInstallPrefix(rel) {
			return nil
		}
		b, err := fsys.ReadFile(p)
		if err != nil {
			return err
		}
		out = append(out, Artifact{RelPath: rel, Content: b})
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Slice(out, func(i, j int) bool { return out[i].RelPath < out[j].RelPath })
	return out, nil
}

var installPrefixes = []string{"agents/", "skills/", "commands/", "phases/"}

func hasInstallPrefix(rel string) bool {
	for _, p := range installPrefixes {
		if strings.HasPrefix(rel, p) {
			return true
		}
	}
	return false
}

// GetRaw reads a specific file from the embedded FS by relative path.
func GetRaw(relPath string) ([]byte, error) {
	return fsys.ReadFile("files/" + relPath)
}

// List returns logical prompt names that should live in the `prompts` table:
// top-level files under files/ + persona-schemas/*.json. Excludes the install
// tree (agents/, skills/, commands/, phases/) — those go via Artifacts().
func List() ([]string, error) {
	var out []string
	// Top-level prompts only.
	entries, err := fsys.ReadDir("files")
	if err != nil {
		return nil, err
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := strings.TrimSuffix(e.Name(), path.Ext(e.Name()))
		out = append(out, name)
	}
	// persona-schemas nested.
	if schemas, err := fsys.ReadDir("files/persona-schemas"); err == nil {
		for _, e := range schemas {
			if e.IsDir() {
				continue
			}
			name := "persona-schema-" + strings.TrimSuffix(e.Name(), path.Ext(e.Name()))
			out = append(out, name)
		}
	}
	sort.Strings(out)
	return out, nil
}
