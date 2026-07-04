//go:build !agt

package policy

import (
	"errors"
	"testing"
)

func TestAGTStubReturnsErrAGTNotCompiled(t *testing.T) {
	got, err := NewAGT(".ai/audit.jsonl")
	if got != nil {
		t.Fatalf("NewAGT() provider = %T, want nil", got)
	}
	if !errors.Is(err, ErrAGTNotCompiled) {
		t.Fatalf("NewAGT() error = %v, want ErrAGTNotCompiled", err)
	}
}
