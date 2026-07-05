// Package cli implements the mivia-agent command surface.
// Plan: WS0. PRD: §1, §4, §9.
package cli

import (
	"bytes"
	"strings"
	"testing"

	"github.com/MiviaLabs/mivia-agentkit/internal/version"
)

func TestRootCommandShowsHelp(t *testing.T) {
	cmd := NewRootCommand()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"--help"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v, want nil", err)
	}

	got := out.String()
	for _, want := range []string{"mivia-agent", "version"} {
		if !strings.Contains(got, want) {
			t.Fatalf("help output missing %q; got %q", want, got)
		}
	}
}

func TestVersionCommandPrintsVersion(t *testing.T) {
	cmd := NewRootCommand()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"version"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v, want nil", err)
	}

	want := "mivia-agent " + version.Version + "\n"
	if got := out.String(); got != want {
		t.Fatalf("version output = %q, want %q", got, want)
	}
}

func TestUnknownCommandExitsNonZero(t *testing.T) {
	cmd := NewRootCommand()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"missing"})

	if err := cmd.Execute(); err == nil {
		t.Fatal("Execute() error = nil, want non-nil")
	}
}

func TestUnknownCommandDoesNotWriteDuplicateError(t *testing.T) {
	cmd := NewRootCommand()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"missing"})

	if err := cmd.Execute(); err == nil {
		t.Fatal("Execute() error = nil, want non-nil")
	}
	if got := out.String(); got != "" {
		t.Fatalf("error output = %q, want empty", got)
	}
}
