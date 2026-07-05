//go:build !agt

package policy

import (
	"errors"
	"testing"
)

func TestFactoryReturnsNoop(t *testing.T) {
	got, err := New("noop", ".ai/audit.jsonl")
	if err != nil {
		t.Fatalf("New() error = %v, want nil", err)
	}
	if got.Name() != "noop" {
		t.Fatalf("Name() = %q, want noop", got.Name())
	}
}

func TestFactoryReturnsErrAGTNotCompiledWithoutTag(t *testing.T) {
	got, err := New("agt", ".ai/audit.jsonl")
	if got != nil {
		t.Fatalf("New() provider = %T, want nil", got)
	}
	if !errors.Is(err, ErrAGTNotCompiled) {
		t.Fatalf("New() error = %v, want ErrAGTNotCompiled", err)
	}
}

func TestFactoryRejectsUnknownProvider(t *testing.T) {
	got, err := New("unknown", ".ai/audit.jsonl")
	if got != nil {
		t.Fatalf("New() provider = %T, want nil", got)
	}
	if err == nil {
		t.Fatalf("New() error = nil, want unknown provider error")
	}
}
