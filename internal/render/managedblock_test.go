// Package render renders embedded templates into target repo files.
// Plan: WS2. PRD: FR-1.2, FR-1.3.
package render

import (
	"bytes"
	"testing"
)

func TestManagedBlockExtractRoundTrip(t *testing.T) {
	content := []byte("pre\n<!-- mivia-agent:managed:start -->\nmanaged\n<!-- mivia-agent:managed:end -->\npost\n")
	pre, managed, post, ok := ExtractManaged(content)
	if !ok {
		t.Fatal("ExtractManaged() ok = false, want true")
	}
	next := append(append(append([]byte{}, pre...), []byte("<!-- mivia-agent:managed:start -->")...), managed...)
	next = append(append(next, []byte("<!-- mivia-agent:managed:end -->")...), post...)
	if !bytes.Equal(next, content) {
		t.Fatalf("round trip = %q, want %q", next, content)
	}
}

func TestManagedBlockUpdatePreservesUserText(t *testing.T) {
	original := []byte("user pre\n<!-- mivia-agent:managed:start -->old<!-- mivia-agent:managed:end -->\nuser post\n")
	got, err := ReplaceManaged(original, []byte("new"))
	if err != nil {
		t.Fatalf("ReplaceManaged() error = %v, want nil", err)
	}
	for _, want := range []string{"user pre", "new", "user post"} {
		if !bytes.Contains(got, []byte(want)) {
			t.Fatalf("ReplaceManaged() = %q, missing %q", got, want)
		}
	}
	if bytes.Contains(got, []byte("old")) {
		t.Fatalf("ReplaceManaged() = %q, still has old managed content", got)
	}
}

func TestManagedBlockNoChangeProducesNoDiff(t *testing.T) {
	original := []byte("# mivia-agent:managed:start\nsame\n# mivia-agent:managed:end\n")
	got, err := ReplaceManaged(original, []byte("\nsame\n"))
	if err != nil {
		t.Fatalf("ReplaceManaged() error = %v, want nil", err)
	}
	if !bytes.Equal(got, original) {
		t.Fatalf("ReplaceManaged() = %q, want no diff", got)
	}
}

func TestManagedBlockRejectsMalformedMarkers(t *testing.T) {
	_, err := ReplaceManaged([]byte("x\n<!-- mivia-agent:managed:start -->\n"), []byte("new"))
	if err == nil {
		t.Fatal("ReplaceManaged() error = nil, want malformed marker error")
	}
}
