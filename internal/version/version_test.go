// Package version verifies release-build metadata injection.
// Plan: WS8. PRD: §9, §11, §14.
package version

import (
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestReleaseVersionInjected(t *testing.T) {
	repo := filepath.Join("..", "..")
	bin := filepath.Join(t.TempDir(), "mivia-agent"+exeSuffix())
	ldflags := "-X github.com/MiviaLabs/mivia-agentkit/internal/version.Version=v9.9.9-test -X github.com/MiviaLabs/mivia-agentkit/internal/version.Commit=abc1234 -X github.com/MiviaLabs/mivia-agentkit/internal/version.Date=2026-07-05T00:00:00Z"
	build := exec.Command("go", "build", "-ldflags", ldflags, "-o", bin, "./cmd/mivia-agent")
	build.Dir = repo
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("go build error = %v, output = %s", err, out)
	}

	out, err := exec.Command(bin, "version").CombinedOutput()
	if err != nil {
		t.Fatalf("%s version error = %v, output = %s", bin, err, out)
	}
	got := string(out)
	if strings.Contains(got, "dev") {
		t.Fatalf("version output = %q, want release version without dev", got)
	}
	if !strings.Contains(got, "v9.9.9-test") {
		t.Fatalf("version output = %q, want injected version", got)
	}
}

func exeSuffix() string {
	if runtime.GOOS == "windows" {
		return ".exe"
	}
	return ""
}
