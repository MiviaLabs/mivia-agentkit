// Package cli implements the mivia-agent command surface.
// Plan: WS13. PRD: FR-4.1, FR-5.3.
package cli

import "strings"

func containsAll(s string, parts ...string) bool {
	for _, part := range parts {
		if !strings.Contains(s, part) {
			return false
		}
	}
	return true
}

func anyContains(items []string, part string) bool {
	for _, item := range items {
		if strings.Contains(item, part) {
			return true
		}
	}
	return false
}
