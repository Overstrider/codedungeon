package reviewpipe

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"text/template"

	"github.com/loldinis/codedungeon/internal/db"
	"github.com/loldinis/codedungeon/internal/prompts"
)

// PromptSource abstracts where Render pulls the template from.
// A *db.Store returns the DB-latest version (so user overrides apply);
// nil falls back to the embedded copy.
type PromptSource interface {
	LatestPrompt(name string) (*db.Prompt, error)
}

// resolvePrompt reads `name` from the store if non-nil, else embedded.
func resolvePrompt(src PromptSource, name string) (string, error) {
	if src != nil {
		p, err := src.LatestPrompt(name)
		if err == nil && p != nil && p.Content != "" {
			return p.Content, nil
		}
	}
	return prompts.Get(name)
}

// RenderView is the data passed to the markdown template.
type RenderView struct {
	Verdict         string
	PersonasStr     string
	ValidatorModel  string
	ClassifierModel string
	StackSpecialist string
	Actionable      []findingView
	DesignDecisions []findingView
	Tally           Tally
	TallyJSON       string
}

type findingView struct {
	Severity                 string
	Title                    string
	File                     string
	LineStart                int
	LineEnd                  int
	FlaggedByStr             string
	EvidenceQuote            string
	SuggestedFix             string
	WhyItMattersOrScenario   string
	ClassifierEvidenceSource string
	ClassifierEvidenceQuote  string
	ClassifierRationale      string
}

// BuildTally counts findings per severity bucket + design decisions.
func BuildTally(findings []Finding, dropped, suppressed int) Tally {
	var t Tally
	for _, f := range findings {
		if f.Actionable {
			switch f.Severity {
			case "P0":
				t.Actionable.P0++
			case "P1":
				t.Actionable.P1++
			case "P2":
				t.Actionable.P2++
			}
		} else if f.DesignDecision {
			t.DesignDecisions++
		}
	}
	t.Dropped = dropped
	t.SuppressedNits = suppressed
	return t
}

// Verdict computes APPROVED / CHANGES_REQUESTED per spec (Step 11).
func Verdict(t Tally) string {
	if t.Actionable.P0 == 0 && t.Actionable.P1 == 0 && t.Actionable.P2 == 0 {
		return "APPROVED"
	}
	return "CHANGES_REQUESTED"
}

// Render produces the review.md body and the review.json content.
// Pass `src` as a *db.Store to honor DB-versioned prompts; pass nil for embedded-only.
func Render(src PromptSource, findings []Finding, tally Tally, verdict string, personas []string, validatorModel, classifierModel, stackSpec string) (md string, review ReviewJSON, err error) {
	if findings == nil {
		findings = []Finding{}
	}
	tpl, err := resolvePrompt(src, "review-md-template")
	if err != nil {
		return "", ReviewJSON{}, err
	}
	t, err := template.New("review").Parse(tpl)
	if err != nil {
		return "", ReviewJSON{}, fmt.Errorf("parse template: %w", err)
	}
	view := RenderView{
		Verdict:         verdict,
		PersonasStr:     strings.Join(personas, " · "),
		ValidatorModel:  validatorModel,
		ClassifierModel: classifierModel,
		StackSpecialist: stackSpec,
		Tally:           tally,
	}
	for _, f := range findings {
		fv := findingView{
			Severity:                 f.Severity,
			Title:                    f.Title,
			File:                     f.File,
			LineStart:                f.LineStart,
			LineEnd:                  f.LineEnd,
			FlaggedByStr:             strings.Join(f.FlaggedBy, ", "),
			EvidenceQuote:            f.EvidenceQuote,
			SuggestedFix:             f.SuggestedFix,
			ClassifierEvidenceSource: f.ClassifierEvidenceSource,
			ClassifierEvidenceQuote:  f.ClassifierEvidenceQuote,
			ClassifierRationale:      f.ClassifierRationale,
		}
		// "Why it matters" = whichever of the context fields is populated.
		switch {
		case f.WhyItMatters != "":
			fv.WhyItMattersOrScenario = f.WhyItMatters
		case f.AttackScenario != "":
			fv.WhyItMattersOrScenario = f.AttackScenario
		case f.ExploitSketch != "":
			fv.WhyItMattersOrScenario = f.ExploitSketch
		case f.SpecClauseQuote != "":
			fv.WhyItMattersOrScenario = "Spec clause: " + f.SpecClauseQuote
		case f.WhySteelmanFails != "":
			fv.WhyItMattersOrScenario = f.WhySteelmanFails
		}
		if f.Actionable {
			view.Actionable = append(view.Actionable, fv)
		} else if f.DesignDecision {
			view.DesignDecisions = append(view.DesignDecisions, fv)
		}
	}
	// TallyJSON for the HTML marker at the end.
	tj, _ := json.Marshal(tally)
	view.TallyJSON = string(tj)

	var buf bytes.Buffer
	if err := t.Execute(&buf, view); err != nil {
		return "", ReviewJSON{}, fmt.Errorf("exec template: %w", err)
	}
	md = buf.String()

	review = ReviewJSON{
		Verdict:         verdict,
		Tally:           tally,
		Findings:        findings,
		PersonasRun:     personas,
		ValidatorModel:  validatorModel,
		ClassifierModel: classifierModel,
		StackSpecialist: stackSpec,
	}
	return md, review, nil
}
