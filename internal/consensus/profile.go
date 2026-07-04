// Package consensus evaluates review verdicts against deterministic voting policy.
// Plan: WS11. PRD: FR-5.2, FR-6.4.
package consensus

import (
	"fmt"
	"strings"
)

// ValidateForProfile validates consensus policy constraints for a profile.
func ValidateForProfile(p Policy, profile string, protectBound bool) error {
	if p.MinReviewers < 1 {
		return fmt.Errorf("min_reviewers must be at least 1")
	}
	if profile != "strict" || !protectBound {
		return nil
	}
	if p.Mode != Majority && p.Mode != Unanimous {
		return fmt.Errorf("strict protected loops require majority or unanimous consensus")
	}
	if p.MinReviewers < 2 {
		return fmt.Errorf("strict protected loops require at least 2 reviewers")
	}
	if strings.HasPrefix(string(p.TieBreaker), string(PreferPrefix)) {
		if _, ok := preferredAdapter(p.TieBreaker); !ok {
			return fmt.Errorf("prefer tie breaker requires adapter name")
		}
	}
	return nil
}
