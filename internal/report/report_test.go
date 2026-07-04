// Package report renders stable mivia-agent findings.
// Plan: WS3. PRD: FR-2.1, FR-2.3.
package report

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestReportTextSortedBySeverity(t *testing.T) {
	got := New([]Finding{
		{Severity: SeverityInfo, Code: "info.late", Path: "z.md", Message: "late"},
		{Severity: SeverityError, Code: "error.first", Path: "b.md", Message: "first"},
		{Severity: SeverityWarn, Code: "warn.second", Path: "a.md", Message: "second"},
	}, false).Text()

	errorAt := strings.Index(got, "error error.first")
	warnAt := strings.Index(got, "warn warn.second")
	infoAt := strings.Index(got, "info info.late")
	if errorAt < 0 || warnAt < 0 || infoAt < 0 || !(errorAt < warnAt && warnAt < infoAt) {
		t.Fatalf("Text() = %q, want severity order error, warn, info", got)
	}
}

func TestReportExitCodeFromFindings(t *testing.T) {
	tests := []struct {
		name     string
		findings []Finding
		strict   bool
		want     int
	}{
		{name: "clean", want: 0},
		{name: "warn non strict", findings: []Finding{{Severity: SeverityWarn}}, want: 0},
		{name: "warn strict", findings: []Finding{{Severity: SeverityWarn}}, strict: true, want: 2},
		{name: "error wins", findings: []Finding{{Severity: SeverityWarn}, {Severity: SeverityError}}, strict: true, want: 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := New(tt.findings, tt.strict).ExitCode; got != tt.want {
				t.Fatalf("ExitCode = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestReportJSONStableOrder(t *testing.T) {
	got, err := New([]Finding{
		{Severity: SeverityWarn, Code: "warn.z", Path: "z.md", Message: "z"},
		{Severity: SeverityError, Code: "error.a", Path: "a.md", Message: "a"},
	}, false).JSON()
	if err != nil {
		t.Fatalf("JSON() error = %v, want nil", err)
	}
	var decoded Report
	if err := json.Unmarshal(got, &decoded); err != nil {
		t.Fatalf("Unmarshal(%s) error = %v", got, err)
	}
	if decoded.Findings[0].Code != "error.a" || decoded.Findings[1].Code != "warn.z" {
		t.Fatalf("JSON findings = %+v, want stable severity order", decoded.Findings)
	}
}
