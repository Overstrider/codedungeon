package reviewpipe

import (
	"testing"
)

func TestLineRangesOverlap(t *testing.T) {
	cases := []struct {
		a0, a1, b0, b1 int
		want           bool
	}{
		{10, 20, 15, 25, true},   // classic overlap
		{10, 20, 20, 25, true},   // touching on edge
		{10, 20, 21, 30, false},  // gap
		{10, 10, 10, 10, true},   // point equal
		{20, 10, 12, 18, true},   // reversed order
	}
	for _, c := range cases {
		got := lineRangesOverlap(c.a0, c.a1, c.b0, c.b1)
		if got != c.want {
			t.Errorf("overlap(%d,%d,%d,%d) = %v, want %v", c.a0, c.a1, c.b0, c.b1, got, c.want)
		}
	}
}

func TestPromote(t *testing.T) {
	if promote("P2") != "P1" {
		t.Errorf("P2→P1 failed")
	}
	if promote("P1") != "P0" {
		t.Errorf("P1→P0 failed")
	}
	if promote("P0") != "P0" {
		t.Errorf("P0→P0 cap failed")
	}
}

func TestDedupeAndPromote_TwoPersonasSameLine(t *testing.T) {
	in := []Finding{
		{Persona: "saboteur", Severity: "P1", File: "a.go", LineStart: 10, LineEnd: 15, FailureClass: "race"},
		{Persona: "security", Severity: "P1", File: "a.go", LineStart: 12, LineEnd: 14, FailureClass: "race"},
	}
	out := DedupeAndPromote(in)
	if len(out) != 1 {
		t.Fatalf("want 1 deduped, got %d", len(out))
	}
	if out[0].Severity != "P0" {
		t.Errorf("severity not promoted: want P0, got %s", out[0].Severity)
	}
	if len(out[0].FlaggedBy) != 2 {
		t.Errorf("flagged_by want 2, got %d", len(out[0].FlaggedBy))
	}
}

func TestDedupeAndPromote_DifferentFiles(t *testing.T) {
	in := []Finding{
		{Persona: "saboteur", Severity: "P1", File: "a.go", LineStart: 10, LineEnd: 15, FailureClass: "race"},
		{Persona: "security", Severity: "P1", File: "b.go", LineStart: 10, LineEnd: 15, FailureClass: "race"},
	}
	out := DedupeAndPromote(in)
	if len(out) != 2 {
		t.Fatalf("want 2 (diff files), got %d", len(out))
	}
	for _, f := range out {
		if f.Severity != "P1" {
			t.Errorf("single-persona should NOT promote, got %s for %s", f.Severity, f.File)
		}
	}
}

func TestDedupeAndPromote_NoOverlapSameFile(t *testing.T) {
	in := []Finding{
		{Persona: "saboteur", Severity: "P1", File: "a.go", LineStart: 10, LineEnd: 15, FailureClass: "race"},
		{Persona: "security", Severity: "P1", File: "a.go", LineStart: 100, LineEnd: 110, FailureClass: "race"},
	}
	out := DedupeAndPromote(in)
	if len(out) != 2 {
		t.Fatalf("want 2 (no line overlap), got %d", len(out))
	}
}

func TestApplyNitCap(t *testing.T) {
	in := []Finding{
		{Severity: "P0"}, {Severity: "P1"},
		{Severity: "P2", Title: "n1"}, {Severity: "P2", Title: "n2"},
		{Severity: "P2", Title: "n3"}, {Severity: "P2", Title: "n4"}, {Severity: "P2", Title: "n5"},
	}
	out, suppressed := ApplyNitCap(in, 3)
	if suppressed != 2 {
		t.Errorf("want 2 suppressed, got %d", suppressed)
	}
	p2Count := 0
	for _, f := range out {
		if f.Severity == "P2" {
			p2Count++
		}
	}
	if p2Count != 3 {
		t.Errorf("want 3 P2, got %d", p2Count)
	}
}

func TestApplyClassifiers_P0HardOverride(t *testing.T) {
	findings := []Finding{{ID: "F1", Severity: "P0"}}
	classifiers := []ClassifierResult{
		{FindingID: "F1", Classification: "design_decision", Confidence: "medium"},
	}
	out := ApplyClassifiers(findings, classifiers)
	if !out[0].Actionable {
		t.Errorf("P0 with medium confidence should be actionable (hard override)")
	}
}

func TestApplyClassifiers_DesignDecisionHighConf(t *testing.T) {
	findings := []Finding{{ID: "F1", Severity: "P1"}}
	classifiers := []ClassifierResult{
		{FindingID: "F1", Classification: "design_decision", Confidence: "high", EvidenceSource: "CLAUDE.md:42"},
	}
	out := ApplyClassifiers(findings, classifiers)
	if out[0].Actionable {
		t.Errorf("high-confidence design_decision should be non-actionable")
	}
	if !out[0].DesignDecision {
		t.Errorf("design_decision flag not set")
	}
}

func TestApplyValidators_DropLowConfidence(t *testing.T) {
	findings := []Finding{
		{ID: "F1"},
		{ID: "F2"},
	}
	validators := []ValidatorResult{
		{FindingID: "F1", Confirmed: true, Confidence: "high"},
		{FindingID: "F2", Confirmed: true, Confidence: "low"},
	}
	kept, dropped := ApplyValidators(findings, validators)
	if len(kept) != 1 {
		t.Errorf("want 1 kept, got %d", len(kept))
	}
	if dropped != 1 {
		t.Errorf("want 1 dropped, got %d", dropped)
	}
}

func TestVerdict(t *testing.T) {
	var t0 Tally
	if v := Verdict(t0); v != "APPROVED" {
		t.Errorf("empty tally should be APPROVED, got %s", v)
	}
	t1 := Tally{}
	t1.Actionable.P2 = 1
	if v := Verdict(t1); v != "CHANGES_REQUESTED" {
		t.Errorf("P2 actionable should block, got %s", v)
	}
}
