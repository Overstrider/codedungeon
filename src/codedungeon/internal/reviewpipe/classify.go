package reviewpipe

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// LoadClassifiers reads all classifier-*.json files from dir (both
// classifier-<idx>.json and classifier-stack-<idx>.json).
func LoadClassifiers(dir string) ([]ClassifierResult, error) {
	var all []ClassifierResult
	patterns := []string{"classifier-*.json"}
	seen := map[string]bool{}
	for _, pat := range patterns {
		paths, err := filepath.Glob(filepath.Join(dir, pat))
		if err != nil {
			return nil, err
		}
		sort.Strings(paths)
		for _, p := range paths {
			if seen[p] {
				continue
			}
			seen[p] = true
			b, err := os.ReadFile(p)
			if err != nil {
				return nil, err
			}
			var c ClassifierResult
			if err := json.Unmarshal(b, &c); err != nil {
				return nil, fmt.Errorf("unmarshal %s: %w", p, err)
			}
			if c.FindingID == "" && c.Idx == 0 {
				base := filepath.Base(p)
				var n int
				// classifier-005.json or classifier-stack-005.json
				if strings.HasPrefix(base, "classifier-stack-") {
					fmt.Sscanf(base, "classifier-stack-%d.json", &n)
				} else {
					fmt.Sscanf(base, "classifier-%d.json", &n)
				}
				c.Idx = n
			}
			all = append(all, c)
		}
	}
	return all, nil
}

// ApplyClassifiers annotates findings as actionable / design_decision based
// on classifier output. Rules:
//   - if classification="design_decision" AND confidence="high" → actionable=false, design_decision=true
//   - else → actionable=true, design_decision=false (safer default)
//   - hard override: severity="P0" AND confidence != "high" → force actionable=true
//
// Findings without a matching classifier default to actionable=true.
func ApplyClassifiers(findings []Finding, classifiers []ClassifierResult) []Finding {
	byID := map[string]ClassifierResult{}
	byIdx := map[int]ClassifierResult{}
	for _, c := range classifiers {
		if c.FindingID != "" {
			byID[c.FindingID] = c
		}
		if c.Idx > 0 {
			byIdx[c.Idx] = c
		}
	}
	for i := range findings {
		f := &findings[i]
		var c ClassifierResult
		var ok bool
		if f.ID != "" {
			if cr, exists := byID[f.ID]; exists {
				c, ok = cr, true
			}
		}
		if !ok {
			if cr, exists := byIdx[i+1]; exists {
				c, ok = cr, true
			}
		}
		if !ok {
			f.Actionable = true
			f.DesignDecision = false
			continue
		}
		if c.Classification == "design_decision" && c.Confidence == "high" {
			f.Actionable = false
			f.DesignDecision = true
			f.ClassifierEvidenceSource = c.EvidenceSource
			f.ClassifierEvidenceQuote = c.EvidenceQuote
			f.ClassifierRationale = c.Rationale
		} else {
			f.Actionable = true
			f.DesignDecision = false
			f.ClassifierRationale = c.Rationale
		}
		// Hard override: P0 without high-confidence design decision stays actionable.
		if f.Severity == "P0" && c.Confidence != "high" {
			f.Actionable = true
			f.DesignDecision = false
		}
	}
	return findings
}
