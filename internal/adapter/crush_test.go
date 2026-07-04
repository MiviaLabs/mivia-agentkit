// Package adapter defines the Crush adapter.
// Plan: WS6. PRD: FR-3.1, FR-3.4.
package adapter

import (
	"context"
	"errors"
	"testing"
)

func TestCrushDetectReportsHeadlessCapability(t *testing.T) {
	r := crushRunner([]byte("crush 0.12.0"), nil)
	d, err := (Crush{Runner: r}).Detect(context.Background())
	if err != nil {
		t.Fatalf("Detect() error = %v", err)
	}
	if d.HeadlessCapable {
		t.Fatalf("HeadlessCapable = true, want false until documented headless mode exists")
	}
	if d.Version != "crush 0.12.0" {
		t.Fatalf("Version = %q, want crush version", d.Version)
	}
}

func TestCrushRunErrorsWhenNotHeadless(t *testing.T) {
	_, err := (Crush{Runner: crushRunner(nil, nil)}).Run(context.Background(), Request{Prompt: "x", Approval: "never"})
	if !errors.Is(err, ErrNotHeadlessCapable) {
		t.Fatalf("Run() error = %v, want ErrNotHeadlessCapable", err)
	}
}

func TestCrushReviewErrorsWhenNotHeadless(t *testing.T) {
	_, err := (Crush{Runner: crushRunner(nil, nil)}).Review(context.Background(), Request{Prompt: "x", Approval: "never"})
	if !errors.Is(err, ErrNotHeadlessCapable) {
		t.Fatalf("Review() error = %v, want ErrNotHeadlessCapable", err)
	}
}

func crushRunner(stdout []byte, err error) *FakeRunner {
	return &FakeRunner{Scripts: map[string]FakeResponse{"crush": {Result: RunResult{Stdout: stdout}, Err: err}}}
}
