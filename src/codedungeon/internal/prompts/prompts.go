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

	"github.com/loldinis/codedungeon/internal/provider"
)

//go:embed all:files
var fsys embed.FS

//go:embed all:codex-files
var codexFS embed.FS

// Get returns the contents of an embedded prompt by logical name.
// Extensions are tried in this order: .md, .tmpl, .json, then literal.
// `persona-schema-X` also resolves to `persona-schemas/X.json`.
func Get(name string) (string, error) {
	return GetFor(provider.Detect().Name(), name)
}

func GetFor(providerName, name string) (string, error) {
	pack := packFor(providerName)
	name = stripNamespace(pack.provider, name)
	candidates := []string{
		pack.root + "/" + name + ".md",
		pack.root + "/" + name + ".tmpl",
		pack.root + "/" + name + ".json",
		pack.root + "/" + name,
	}
	if strings.HasPrefix(name, "persona-schema-") {
		candidates = append(candidates, pack.root+"/persona-schemas/"+strings.TrimPrefix(name, "persona-schema-")+".json")
	}
	for _, c := range candidates {
		b, err := pack.fs.ReadFile(c)
		if err == nil {
			return string(b), nil
		}
	}
	if pack.provider != "claude" {
		return GetFor("claude", name)
	}
	return "", fmt.Errorf("embedded prompt not found: %s", name)
}

// Artifact is one file from the embedded tree, ready to be written into
// <project>/.claude/<RelPath>.
type Artifact struct {
	RelPath     string // pack-relative path, e.g. "agents/cd_dev_worker.toml"
	InstallPath string // project-relative path, e.g. ".codex/agents/cd_dev_worker.toml"
	Content     []byte
	Provider    string
	PackID      string
	PackVersion string
	Kind        string
	LogicalName string
}

// Artifacts walks the embedded tree and returns every file under
// files/{agents,skills,commands,phases}/. Everything else (caveman, handoff,
// persona-schemas, templates at root) stays embedded-only (Get()-accessed).
func Artifacts() ([]Artifact, error) {
	return ArtifactsFor(provider.Detect().Name())
}

func ArtifactsFor(providerName string) ([]Artifact, error) {
	pack := packFor(providerName)
	var out []Artifact
	err := fs.WalkDir(pack.fs, pack.root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		rel := strings.TrimPrefix(p, pack.root+"/")
		installPath, ok := pack.installPath(rel)
		if !ok {
			return nil
		}
		b, err := pack.fs.ReadFile(p)
		if err != nil {
			return err
		}
		out = append(out, Artifact{
			RelPath:     rel,
			InstallPath: installPath,
			Content:     b,
			Provider:    pack.provider,
			PackID:      pack.id,
			PackVersion: pack.version,
			Kind:        artifactKind(rel),
			LogicalName: logicalName(rel),
		})
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
	return GetRawFor(provider.Detect().Name(), relPath)
}

func GetRawFor(providerName, relPath string) ([]byte, error) {
	pack := packFor(providerName)
	b, err := pack.fs.ReadFile(pack.root + "/" + relPath)
	if err == nil {
		return b, nil
	}
	if pack.provider != "claude" {
		return GetRawFor("claude", relPath)
	}
	return nil, err
}

// List returns logical prompt names that should live in the `prompts` table:
// top-level files under files/ + persona-schemas/*.json. Excludes the install
// tree (agents/, skills/, commands/, phases/) — those go via Artifacts().
func List() ([]string, error) {
	return ListFor(provider.Detect().Name())
}

func ListFor(providerName string) ([]string, error) {
	pack := packFor(providerName)
	var out []string
	// Top-level prompts only.
	entries, err := pack.fs.ReadDir(pack.root)
	if err != nil {
		return nil, err
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if _, ok := pack.installPath(e.Name()); ok {
			continue
		}
		name := namespace(pack.provider, strings.TrimSuffix(e.Name(), path.Ext(e.Name())))
		out = append(out, name)
	}
	// persona-schemas nested.
	if schemas, err := pack.fs.ReadDir(pack.root + "/persona-schemas"); err == nil {
		for _, e := range schemas {
			if e.IsDir() {
				continue
			}
			name := namespace(pack.provider, "persona-schema-"+strings.TrimSuffix(e.Name(), path.Ext(e.Name())))
			out = append(out, name)
		}
	}
	sort.Strings(out)
	return out, nil
}

type pack struct {
	provider string
	id       string
	version  string
	root     string
	fs       embed.FS
}

func packFor(providerName string) pack {
	switch providerName {
	case "codex", "codex-cli":
		return pack{provider: "codex", id: "codedungeon-codex", version: "1", root: "codex-files", fs: codexFS}
	default:
		return pack{provider: "claude", id: "codedungeon-claude", version: "1", root: "files", fs: fsys}
	}
}

func (p pack) installPath(rel string) (string, bool) {
	if p.provider == "claude" {
		if !hasInstallPrefix(rel) {
			return "", false
		}
		return ".claude/" + rel, true
	}
	switch {
	case rel == "AGENTS.md":
		return rel, true
	case rel == "config.toml":
		return ".codex/config.toml", true
	case strings.HasPrefix(rel, "agents/"):
		return ".codex/" + rel, true
	case strings.HasPrefix(rel, "commands/"):
		return ".codex/" + rel, true
	case strings.HasPrefix(rel, "phases/"):
		return ".codex/" + rel, true
	case strings.HasPrefix(rel, "skills/"):
		return ".agents/" + rel, true
	default:
		return "", false
	}
}

func namespace(providerName, name string) string {
	if providerName == "claude" || strings.Contains(name, ":") {
		return name
	}
	return providerName + ":" + name
}

func stripNamespace(providerName, name string) string {
	prefix := providerName + ":"
	return strings.TrimPrefix(name, prefix)
}

func artifactKind(rel string) string {
	switch {
	case rel == "AGENTS.md":
		return "project-instructions"
	case rel == "config.toml":
		return "provider-config"
	case strings.HasPrefix(rel, "agents/"):
		return "agent"
	case strings.HasPrefix(rel, "skills/"):
		return "skill"
	case strings.HasPrefix(rel, "commands/"):
		return "command"
	case strings.HasPrefix(rel, "phases/"):
		return "phase"
	default:
		return "artifact"
	}
}

func logicalName(rel string) string {
	name := path.Base(rel)
	return strings.TrimSuffix(name, path.Ext(name))
}
