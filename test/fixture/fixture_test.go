// Package fixture runs end-to-end generated-fixture smoke coverage.
// Plan: WS8. PRD: §9, §11, §14.
package fixture

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"sort"
	"strings"
	"testing"
)

func TestGeneratedFixtureDoctorPasses(t *testing.T) {
	env := newFixtureEnv(t)

	doctor := env.run(t, true, "doctor", "--repo", env.repo, "--json")
	var report struct {
		Findings []struct {
			Severity string `json:"severity"`
		} `json:"findings"`
		ExitCode int `json:"exit_code"`
	}
	decodeJSON(t, doctor.Stdout, &report)
	if report.ExitCode != 0 {
		t.Fatalf("doctor exit_code = %d, want 0; stdout=%s stderr=%s", report.ExitCode, doctor.Stdout, doctor.Stderr)
	}

	audit := env.run(t, true, "audit", "--repo", env.repo, "--json")
	var auditReport struct {
		Findings []struct {
			Severity string `json:"severity"`
		} `json:"findings"`
	}
	decodeJSON(t, audit.Stdout, &auditReport)
	for _, finding := range auditReport.Findings {
		if finding.Severity == "error" {
			t.Fatalf("audit findings include error severity; stdout=%s stderr=%s", audit.Stdout, audit.Stderr)
		}
	}

	preflight := env.run(t, true,
		"preflight",
		"--repo", env.repo,
		"--contract-row", "fixture",
		"--focused-verifier", "go test ./test/fixture/... -count=1",
		"--mutation-proof", "fixture expected file set mismatch fails",
		"--json",
	)
	var stamp map[string]any
	decodeJSON(t, preflight.Stdout, &stamp)
	if _, err := os.Stat(filepath.Join(env.repo, ".git", "mivia-agent-quality-stamp.json")); err != nil {
		t.Fatalf("quality stamp missing: %v", err)
	}
}

func TestGeneratedFixtureFileSetMatchesStandardProfile(t *testing.T) {
	env := newFixtureEnv(t)

	actual := repoFiles(t, env.repo)
	sort.Strings(actual)
	sort.Strings(env.expectedFiles)
	if !reflect.DeepEqual(actual, env.expectedFiles) {
		t.Fatalf("repo files mismatch\nactual=%v\nwant=%v", actual, env.expectedFiles)
	}
}

func TestGeneratedFixtureDryRunPlanNonEmpty(t *testing.T) {
	env := newFixtureEnv(t)

	adapters := env.run(t, false, "adapters", "--repo", env.repo, "--json")
	var statuses []struct {
		Name string `json:"name"`
	}
	decodeJSON(t, adapters.Stdout, &statuses)
	for _, want := range []string{"codex", "claude"} {
		if !hasAdapter(statuses, want) {
			t.Fatalf("adapters output missing %q: stdout=%s stderr=%s", want, adapters.Stdout, adapters.Stderr)
		}
	}

	plan := env.run(t, true, "run", "--repo", env.repo, "--workflow", "research", "--dry-run", "--json")
	var rows []map[string]any
	decodeJSON(t, plan.Stdout, &rows)
	if len(rows) == 0 {
		t.Fatalf("dry-run plan empty: stdout=%s stderr=%s", plan.Stdout, plan.Stderr)
	}
}

type fixtureEnv struct {
	binary        string
	repo          string
	home          string
	expectedFiles []string
}

type commandResult struct {
	Stdout   string
	Stderr   string
	ExitCode int
}

func newFixtureEnv(t *testing.T) fixtureEnv {
	t.Helper()
	home := t.TempDir()
	repo := tempGitRepo(t)
	binary := buildBinary(t)

	preview := runCommand(t, binary, repo, home, true,
		"init",
		"--repo", repo,
		"--profile", "standard",
		"--adapter", "codex",
		"--adapter", "claude",
		"--adapter", "copilot",
		"--dry-run",
		"--json",
	)
	var planned struct {
		FilesCreated []string `json:"files_created"`
	}
	decodeJSON(t, preview.Stdout, &planned)

	runCommand(t, binary, repo, home, true,
		"init",
		"--repo", repo,
		"--profile", "standard",
		"--adapter", "codex",
		"--adapter", "claude",
		"--adapter", "copilot",
		"--write",
	)

	return fixtureEnv{
		binary:        binary,
		repo:          repo,
		home:          home,
		expectedFiles: append([]string(nil), planned.FilesCreated...),
	}
}

func (e fixtureEnv) run(t *testing.T, expectZero bool, args ...string) commandResult {
	t.Helper()
	return runCommand(t, e.binary, e.repo, e.home, expectZero, args...)
}

func buildBinary(t *testing.T) string {
	t.Helper()
	bin := filepath.Join(t.TempDir(), "mivia-agent"+exeSuffix())
	cmd := exec.Command("go", "build", "-o", bin, "./cmd/mivia-agent")
	cmd.Dir = filepath.Join("..", "..")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("go build error = %v, output = %s", err, out)
	}
	return bin
}

func runCommand(t *testing.T, binary, repo, home string, expectZero bool, args ...string) commandResult {
	t.Helper()
	cmd := exec.Command(binary, args...)
	cmd.Dir = repo
	cmd.Env = append(os.Environ(), "HOME="+home, "USERPROFILE="+home)
	out, err := cmd.Output()
	result := commandResult{Stdout: string(out)}
	if err == nil {
		return result
	}
	if exitErr, ok := err.(*exec.ExitError); ok {
		result.Stderr = string(exitErr.Stderr)
		result.ExitCode = exitErr.ExitCode()
		if expectZero {
			t.Fatalf("%s %s exit=%d stdout=%s stderr=%s", binary, strings.Join(args, " "), result.ExitCode, result.Stdout, result.Stderr)
		}
		return result
	}
	t.Fatalf("%s %s error = %v", binary, strings.Join(args, " "), err)
	return commandResult{}
}

func tempGitRepo(t *testing.T) string {
	t.Helper()
	repo := t.TempDir()
	runGit(t, repo, "init", "-q")
	runGit(t, repo, "config", "user.email", "test@example.invalid")
	runGit(t, repo, "config", "user.name", "Test User")
	runGit(t, repo, "commit", "-q", "--allow-empty", "-m", "init")
	return repo
}

func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v error = %v, output = %s", args, err, out)
	}
}

func repoFiles(t *testing.T, repo string) []string {
	t.Helper()
	var files []string
	if err := filepath.WalkDir(repo, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() && d.Name() == ".git" {
			return filepath.SkipDir
		}
		if d.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(repo, path)
		if err != nil {
			return err
		}
		files = append(files, filepath.ToSlash(rel))
		return nil
	}); err != nil {
		t.Fatalf("WalkDir() error = %v", err)
	}
	return files
}

func decodeJSON(t *testing.T, data string, dest any) {
	t.Helper()
	if err := json.Unmarshal([]byte(data), dest); err != nil {
		t.Fatalf("Unmarshal(%q) error = %v", data, err)
	}
}

func hasAdapter(statuses []struct {
	Name string `json:"name"`
}, want string) bool {
	for _, status := range statuses {
		if status.Name == want {
			return true
		}
	}
	return false
}

func exeSuffix() string {
	if runtime.GOOS == "windows" {
		return ".exe"
	}
	return ""
}
