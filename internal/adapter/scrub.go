// Package adapter defines headless CLI adapter contracts.
// Plan: WS9. PRD: FR-3.1, FR-3.2, FR-7.4.
package adapter

import "regexp"

type secretPattern struct {
	kind string
	re   *regexp.Regexp
}

var secretPatterns = []secretPattern{
	{kind: "aws", re: regexp.MustCompile(`AKIA[0-9A-Z]{16}`)},
	{kind: "bearer", re: regexp.MustCompile(`Bearer\s+[A-Za-z0-9._~+/=-]{12,}`)},
	{kind: "github", re: regexp.MustCompile(`gh[pousr]_[A-Za-z0-9]{36,}`)},
	{kind: "jwt", re: regexp.MustCompile(`eyJ[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+`)},
	{kind: "env", re: regexp.MustCompile(`(?i)(SECRET|TOKEN|PASSWORD|API_KEY)=\S+`)},
}

// Scrub redacts common secret-shaped values.
func Scrub(b []byte) []byte {
	out := string(b)
	for _, p := range secretPatterns {
		out = p.re.ReplaceAllString(out, "<redacted:"+p.kind+">")
	}
	return []byte(out)
}
