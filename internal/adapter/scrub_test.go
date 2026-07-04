// Package adapter defines headless CLI adapter contracts.
// Plan: WS9. PRD: FR-3.1, FR-3.2, FR-7.4.
package adapter

import (
	"bytes"
	"strings"
	"testing"
)

func TestScrubAWSKey(t *testing.T) {
	got := Scrub([]byte("key " + fakeAWSKey() + " here"))
	if !bytes.Contains(got, []byte("<redacted:aws>")) {
		t.Fatalf("Scrub() = %q, want aws redaction", got)
	}
}

func TestScrubBearerToken(t *testing.T) {
	got := Scrub([]byte("Authorization: Bearer " + strings.Repeat("a", 16)))
	if !bytes.Contains(got, []byte("<redacted:bearer>")) {
		t.Fatalf("Scrub() = %q, want bearer redaction", got)
	}
}

func TestScrubGitHubPAT(t *testing.T) {
	got := Scrub([]byte("ghp_" + strings.Repeat("a", 36)))
	if !bytes.Contains(got, []byte("<redacted:github>")) {
		t.Fatalf("Scrub() = %q, want github redaction", got)
	}
}

func TestScrubEnvAssignment(t *testing.T) {
	got := Scrub([]byte("API_KEY=" + strings.Repeat("a", 16)))
	if !bytes.Contains(got, []byte("<redacted:env>")) {
		t.Fatalf("Scrub() = %q, want env redaction", got)
	}
}

func TestScrubIdempotent(t *testing.T) {
	once := Scrub([]byte("TOKEN=" + strings.Repeat("a", 16)))
	twice := Scrub(once)
	if !bytes.Equal(once, twice) {
		t.Fatalf("Scrub twice = %q, want %q", twice, once)
	}
}

func fakeAWSKey() string {
	return "AKIA" + strings.Repeat("A", 16)
}

func TestScrubLeavesNonSecrets(t *testing.T) {
	in := []byte("ordinary output")
	if got := Scrub(in); !bytes.Equal(got, in) {
		t.Fatalf("Scrub() = %q, want %q", got, in)
	}
}
