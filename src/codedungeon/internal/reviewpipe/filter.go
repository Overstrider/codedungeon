package reviewpipe

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
)

// LoadValidators reads all validator-*.json files from dir.
func LoadValidators(dir string) ([]ValidatorResult, error) {
	paths, err := filepath.Glob(filepath.Join(dir, "validator-*.json"))
	if err != nil {
		return nil, err
	}
	sort.Strings(paths)
	var out []ValidatorResult
	for _, p := range paths {
		b, err := os.ReadFile(p)
		if err != nil {
			return nil, err
		}
		var v ValidatorResult
		if err := json.Unmarshal(b, &v); err != nil {
			return nil, fmt.Errorf("unmarshal %s: %w", p, err)
		}
		// If idx missing, derive from filename suffix.
		if v.FindingID == "" && v.Idx == 0 {
			// validator-005.json → 5
			base := filepath.Base(p)
			var n int
			if _, err := fmt.Sscanf(base, "validator-%d.json", &n); err == nil {
				v.Idx = n
			}
		}
		out = append(out, v)
	}
	return out, nil
}

// ApplyValidators filters findings by validator results. A finding survives
// only if at least one matching validator reports confirmed=true and
// confidence ≠ "low". Drops rise in the returned `dropped` count.
func ApplyValidators(findings []Finding, validators []ValidatorResult) (kept []Finding, dropped int) {
	// Index validators by both ID and by index (1-based to match AssignIDs or filename idx).
	byID := map[string]ValidatorResult{}
	byIdx := map[int]ValidatorResult{}
	for _, v := range validators {
		if v.FindingID != "" {
			byID[v.FindingID] = v
		}
		if v.Idx > 0 {
			byIdx[v.Idx] = v
		}
	}
	for i, f := range findings {
		// Lookup: try ID first, else fall back to 1-based position.
		var v ValidatorResult
		var ok bool
		if f.ID != "" {
			if vr, exists := byID[f.ID]; exists {
				v, ok = vr, true
			}
		}
		if !ok {
			if vr, exists := byIdx[i+1]; exists {
				v, ok = vr, true
			}
		}
		// No validator found → conservative: keep (validator pass incomplete).
		if !ok {
			kept = append(kept, f)
			continue
		}
		if !v.Confirmed || v.Confidence == "low" {
			dropped++
			continue
		}
		f.Confirmed = ptrBool(v.Confirmed)
		f.Confidence = v.Confidence
		kept = append(kept, f)
	}
	return kept, dropped
}

func ptrBool(b bool) *bool { return &b }
