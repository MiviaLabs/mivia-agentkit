// Package consensus evaluates review verdicts against deterministic voting policy.
// Plan: WS11. PRD: FR-5.2, FR-6.4.
package consensus

import "testing"

func TestStrictProtectBoundRejectsFirstPass(t *testing.T) {
	err := ValidateForProfile(Policy{Mode: FirstPass, MinReviewers: 2}, "strict", true)
	if err == nil {
		t.Fatalf("ValidateForProfile() error = nil, want first-pass rejection")
	}
}

func TestStrictProtectBoundRejectsMinReviewersOne(t *testing.T) {
	err := ValidateForProfile(Policy{Mode: Majority, MinReviewers: 1}, "strict", true)
	if err == nil {
		t.Fatalf("ValidateForProfile() error = nil, want min_reviewers rejection")
	}
}

func TestStrictProtectBoundAcceptsMajorityMinReviewersTwo(t *testing.T) {
	err := ValidateForProfile(Policy{Mode: Majority, MinReviewers: 2}, "strict", true)
	if err != nil {
		t.Fatalf("ValidateForProfile() error = %v, want nil", err)
	}
}

func TestStrictProtectBoundRejectsBarePreferTieBreaker(t *testing.T) {
	err := ValidateForProfile(Policy{Mode: Majority, MinReviewers: 2, TieBreaker: PreferPrefix}, "strict", true)
	if err == nil {
		t.Fatalf("ValidateForProfile() error = nil, want bare prefer rejection")
	}
}

func TestNonStrictAcceptsFirstPass(t *testing.T) {
	err := ValidateForProfile(Policy{Mode: FirstPass, MinReviewers: 1}, "standard", true)
	if err != nil {
		t.Fatalf("ValidateForProfile() error = %v, want nil", err)
	}
}
