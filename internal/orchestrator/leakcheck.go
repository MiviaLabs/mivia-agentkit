// Package orchestrator resolves and executes bounded agent loops.
// Plan: WS10. PRD: FR-7.4.
package orchestrator

import (
	"os"
	"path/filepath"
	"regexp"
)

var leakPatterns = []*regexp.Regexp{
	regexp.MustCompile(`AKIA[0-9A-Z]{16}`),
	regexp.MustCompile(`Bearer\s+[A-Za-z0-9._~+/=-]{12,}`),
	regexp.MustCompile(`gh[pousr]_[A-Za-z0-9]{36,}`),
	regexp.MustCompile(`eyJ[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+`),
	regexp.MustCompile(`(?i)(SECRET|TOKEN|PASSWORD|API_KEY)=\S+`),
	regexp.MustCompile(`(?i)"?(prompt|completion|content)"?\s*:`),
}

// AssertNoLeaks fails t if dir contains raw prompts, outputs, or secrets.
func AssertNoLeaks(t leakTester, dir string) {
	t.Helper()
	err := filepath.WalkDir(dir, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		for _, pattern := range leakPatterns {
			if pattern.Match(data) {
				t.Fatalf("leak pattern %q found in %s", pattern.String(), path)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("scan leaks: %v", err)
	}
}

type leakTester interface {
	Helper()
	Fatalf(string, ...any)
}
