// Package render renders embedded templates into target repo files.
// Plan: WS2. PRD: FR-1.1, FR-1.2, FR-10.6.
package render

import (
	"strings"
	"testing"
)

func TestRenderFillsProjectName(t *testing.T) {
	got, err := New().Render("core/INDEX.md.tmpl", Vars{Project: ProjectVars{Name: "demo"}, Profile: "standard"})
	if err != nil {
		t.Fatalf("Render() error = %v, want nil", err)
	}
	if !strings.Contains(string(got), "demo") {
		t.Fatalf("Render() = %q, want project name", got)
	}
}

func TestRenderErrorsOnUndefinedReferencedVar(t *testing.T) {
	_, err := New().Render("core/broken-undefined.md.tmpl", Vars{})
	if err == nil || !strings.Contains(err.Error(), "Missing") {
		t.Fatalf("Render() error = %v, want missing key error", err)
	}
}

func TestRenderAllProducesAllExpectedOutputs(t *testing.T) {
	plan := RenderPlan{
		{Template: "core/INDEX.md.tmpl", OutPath: ".ai/INDEX.md"},
		{Template: "core/rules/00-operating-doctrine.md.tmpl", OutPath: ".ai/rules/00-operating-doctrine.md"},
	}
	got, err := New().RenderAll(plan, Vars{Project: ProjectVars{Name: "demo"}, Profile: "standard"})
	if err != nil {
		t.Fatalf("RenderAll() error = %v, want nil", err)
	}
	for _, want := range []string{".ai/INDEX.md", ".ai/rules/00-operating-doctrine.md"} {
		if len(got[want]) == 0 {
			t.Fatalf("RenderAll()[%q] empty; got %#v", want, got)
		}
	}
}
