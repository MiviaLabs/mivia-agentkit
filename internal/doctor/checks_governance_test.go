//go:build !agt

package doctor

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/MiviaLabs/mivia-agentkit/internal/report"
)

func TestDoctorFailsWhenStrictRequiresAGTButUnavailable(t *testing.T) {
	repo, home := freshRepo(t)
	data := readFile(t, filepath.Join(repo, "mivia-agent.yaml"))
	data = strings.Replace(data, "profile: standard", "profile: strict", 1)
	data = strings.Replace(data, "provider: noop", "provider: agt", 1)
	writeFile(t, filepath.Join(repo, "mivia-agent.yaml"), data)

	got := Run(Context{Repo: repo, GlobalDir: filepath.Join(home, ".agents")})
	assertCode(t, got, "governance.agt_required_unavailable")
	if got.ExitCode != 1 {
		t.Fatalf("ExitCode = %d, want 1 for strict AGT unavailable", got.ExitCode)
	}
	if !hasSeverity(got.Findings, report.SeverityError) {
		t.Fatalf("findings = %+v, want error severity", got.Findings)
	}
}
