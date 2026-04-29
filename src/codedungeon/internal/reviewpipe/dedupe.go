package reviewpipe

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// CanonicalPersona maps historical persona names to the short file names used
// by the review pipeline.
func CanonicalPersona(name string) string {
	switch strings.TrimSpace(name) {
	case "security_auditor":
		return "security"
	case "spec_enforcer":
		return "spec"
	default:
		return strings.TrimSpace(name)
	}
}

// PersonaFileCandidates returns accepted file suffixes for a persona.
func PersonaFileCandidates(name string) []string {
	switch CanonicalPersona(name) {
	case "security":
		return []string{"security", "security_auditor"}
	case "spec":
		return []string{"spec", "spec_enforcer"}
	default:
		return []string{CanonicalPersona(name)}
	}
}

// PersonaFindingsPath returns the first existing findings file for a persona.
func PersonaFindingsPath(dir, name string) (string, bool) {
	for _, candidate := range PersonaFileCandidates(name) {
		path := filepath.Join(dir, "findings-"+candidate+".json")
		if _, err := os.Stat(path); err == nil {
			return path, true
		}
	}
	return filepath.Join(dir, "findings-"+CanonicalPersona(name)+".json"), false
}

// LoadPersonaFindings reads all findings-<persona>.json files from dir.
// Expected names: findings-saboteur.json, findings-newhire.json,
// findings-security.json, findings-spec.json (+ optional findings-stack.json).
func LoadPersonaFindings(dir string) ([]Finding, []string, error) {
	names := []string{"saboteur", "newhire", "security", "spec"}
	var all []Finding
	var loaded []string
	for _, n := range names {
		path, ok := PersonaFindingsPath(dir, n)
		if !ok {
			continue
		}
		b, err := os.ReadFile(path)
		if err != nil {
			if os.IsNotExist(err) {
				// Persona missing is non-fatal — log via caller.
				continue
			}
			return nil, nil, fmt.Errorf("read %s: %w", path, err)
		}
		var pf PersonaFile
		if err := json.Unmarshal(b, &pf); err != nil {
			// Try plain array fallback.
			var arr []Finding
			if aerr := json.Unmarshal(b, &arr); aerr == nil {
				pf = PersonaFile{Persona: n, Findings: arr}
			} else {
				return nil, nil, fmt.Errorf("unmarshal %s: %w", path, err)
			}
		}
		for i := range pf.Findings {
			if pf.Findings[i].Persona == "" {
				pf.Findings[i].Persona = n
			} else {
				pf.Findings[i].Persona = CanonicalPersona(pf.Findings[i].Persona)
			}
			if pf.Findings[i].Source == "" {
				pf.Findings[i].Source = "persona"
			}
		}
		all = append(all, pf.Findings...)
		loaded = append(loaded, n)
	}
	return all, loaded, nil
}

// LoadStackFindings reads findings-stack.json (stack-specialist's additions).
func LoadStackFindings(dir string) ([]Finding, error) {
	path := filepath.Join(dir, "findings-stack.json")
	b, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var arr []Finding
	if err := json.Unmarshal(b, &arr); err != nil {
		var pf PersonaFile
		if err2 := json.Unmarshal(b, &pf); err2 != nil {
			return nil, err
		}
		arr = pf.Findings
	}
	for i := range arr {
		if arr[i].Source == "" {
			arr[i].Source = "lang-specialist"
		}
	}
	return arr, nil
}

// lineRangesOverlap returns true if [a0,a1] and [b0,b1] touch by ≥ 1 line.
func lineRangesOverlap(a0, a1, b0, b1 int) bool {
	if a1 < a0 {
		a0, a1 = a1, a0
	}
	if b1 < b0 {
		b0, b1 = b1, b0
	}
	return a0 <= b1 && b0 <= a1
}

// categoryKey derives the dedupe category from persona-specific fields.
func categoryKey(f Finding) string {
	switch {
	case f.FailureClass != "":
		return "saboteur:" + f.FailureClass
	case f.Category != "":
		return "category:" + f.Category
	}
	return f.Persona
}

// DedupeAndPromote groups findings by (file, category-ish) with line-range
// overlap, merges them, and promotes severity by one tier per additional persona
// (capped at P0). Returns deduped findings with FlaggedBy populated.
func DedupeAndPromote(in []Finding) []Finding {
	// Sort: lowest severity rank first (actually highest importance first)
	// so the first finding kept per group has the highest severity.
	sort.SliceStable(in, func(i, j int) bool {
		return sevRank(in[i].Severity) < sevRank(in[j].Severity)
	})

	type group struct {
		base      *Finding
		personas  map[string]struct{}
		collected []Finding
	}
	var groups []*group
	for i := range in {
		f := &in[i]
		var matched *group
		for _, g := range groups {
			if g.base.File != f.File {
				continue
			}
			if categoryKey(*g.base) != categoryKey(*f) {
				continue
			}
			if !lineRangesOverlap(g.base.LineStart, g.base.LineEnd, f.LineStart, f.LineEnd) {
				continue
			}
			matched = g
			break
		}
		if matched == nil {
			ng := &group{
				base:     f,
				personas: map[string]struct{}{f.Persona: {}},
			}
			ng.collected = append(ng.collected, *f)
			groups = append(groups, ng)
			continue
		}
		matched.personas[f.Persona] = struct{}{}
		matched.collected = append(matched.collected, *f)
		// Widen line range to union.
		if f.LineStart < matched.base.LineStart {
			matched.base.LineStart = f.LineStart
		}
		if f.LineEnd > matched.base.LineEnd {
			matched.base.LineEnd = f.LineEnd
		}
	}

	out := make([]Finding, 0, len(groups))
	for _, g := range groups {
		f := *g.base
		// FlaggedBy = sorted list of personas.
		personas := make([]string, 0, len(g.personas))
		for p := range g.personas {
			personas = append(personas, p)
		}
		sort.Strings(personas)
		f.FlaggedBy = personas
		// Severity promotion: +1 tier per extra persona (cap P0).
		if len(personas) >= 2 {
			f.Severity = promote(f.Severity)
			for i := 2; i < len(personas); i++ {
				f.Severity = promote(f.Severity)
			}
		}
		out = append(out, f)
	}

	// Sort deterministically by severity then file:line.
	sort.SliceStable(out, func(i, j int) bool {
		ri, rj := sevRank(out[i].Severity), sevRank(out[j].Severity)
		if ri != rj {
			return ri < rj
		}
		if out[i].File != out[j].File {
			return out[i].File < out[j].File
		}
		return out[i].LineStart < out[j].LineStart
	})
	return out
}

// ApplyNitCap keeps at most `cap` P2 findings total; extras roll into
// suppressedNits count and are removed.
func ApplyNitCap(in []Finding, cap int) (out []Finding, suppressed int) {
	if cap < 0 {
		return in, 0
	}
	kept := 0
	for _, f := range in {
		if f.Severity != "P2" {
			out = append(out, f)
			continue
		}
		if kept < cap {
			out = append(out, f)
			kept++
		} else {
			suppressed++
		}
	}
	return out, suppressed
}

// AssignIDs populates Finding.ID when empty. Stable hash-ish by position.
func AssignIDs(in []Finding) []Finding {
	for i := range in {
		if in[i].ID == "" {
			in[i].ID = fmt.Sprintf("F%03d", i+1)
		}
	}
	return in
}

// Truncate is a small helper used by tests.
func Truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return strings.TrimRight(s[:n], " ") + "…"
}
