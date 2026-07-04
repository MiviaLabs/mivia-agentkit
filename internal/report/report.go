// Package report renders stable mivia-agent findings.
// Plan: WS3. PRD: FR-2.1, FR-2.3.
package report

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

// Severity names the finding severity.
type Severity string

const (
	// SeverityError fails doctor and strict audit gates.
	SeverityError Severity = "error"
	// SeverityWarn is advisory unless strict mode is enabled.
	SeverityWarn Severity = "warn"
	// SeverityInfo is informational.
	SeverityInfo Severity = "info"
)

// Finding is one stable diagnostic row.
type Finding struct {
	Severity Severity `json:"severity"`
	Code     string   `json:"code"`
	Message  string   `json:"message"`
	Path     string   `json:"path,omitempty"`
}

// Report is the stable output contract for validators and audits.
type Report struct {
	Findings []Finding `json:"findings"`
	ExitCode int       `json:"exit_code"`
}

// New builds a report with deterministic finding order and exit code.
func New(findings []Finding, strict bool) Report {
	sorted := append([]Finding(nil), findings...)
	sortFindings(sorted)
	return Report{Findings: sorted, ExitCode: ExitCode(sorted, strict)}
}

// ExitCode returns 1 for errors, 2 for strict warn-only reports, and 0 otherwise.
func ExitCode(findings []Finding, strict bool) int {
	hasWarn := false
	for _, finding := range findings {
		if finding.Severity == SeverityError {
			return 1
		}
		if finding.Severity == SeverityWarn {
			hasWarn = true
		}
	}
	if strict && hasWarn {
		return 2
	}
	return 0
}

// Text renders concise stable lines sorted by severity then path.
func (r Report) Text() string {
	findings := append([]Finding(nil), r.Findings...)
	sortFindings(findings)
	if len(findings) == 0 {
		return "ok\n"
	}
	var b strings.Builder
	for _, finding := range findings {
		path := finding.Path
		if path == "" {
			path = "-"
		}
		fmt.Fprintf(&b, "%s %s %s: %s\n", finding.Severity, finding.Code, path, finding.Message)
	}
	return b.String()
}

// JSON renders deterministic indented JSON.
func (r Report) JSON() ([]byte, error) {
	out := Report{Findings: append([]Finding(nil), r.Findings...), ExitCode: r.ExitCode}
	sortFindings(out.Findings)
	return json.MarshalIndent(out, "", "  ")
}

func sortFindings(findings []Finding) {
	sort.SliceStable(findings, func(i, j int) bool {
		left := severityRank(findings[i].Severity)
		right := severityRank(findings[j].Severity)
		if left != right {
			return left < right
		}
		if findings[i].Path != findings[j].Path {
			return findings[i].Path < findings[j].Path
		}
		return findings[i].Code < findings[j].Code
	})
}

func severityRank(severity Severity) int {
	switch severity {
	case SeverityError:
		return 0
	case SeverityWarn:
		return 1
	case SeverityInfo:
		return 2
	default:
		return 3
	}
}
