//go:build agt

package policy

import (
	"errors"
	"testing"
)

func TestAGTProviderFailsClosedUntilSDKPinned(t *testing.T) {
	got, err := NewAGT(".ai/audit.jsonl")
	if got != nil {
		t.Fatalf("NewAGT() provider = %T, want nil", got)
	}
	if !errors.Is(err, ErrAGTSDKUnavailable) {
		t.Fatalf("NewAGT() error = %v, want ErrAGTSDKUnavailable", err)
	}
}
