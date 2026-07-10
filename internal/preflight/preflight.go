// Package preflight writes and validates quality stamps.
// Plan: WS4. PRD: FR-2.4, FR-7.1.
package preflight

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/MiviaLabs/mivia-agentkit/internal/config"
	"github.com/MiviaLabs/mivia-agentkit/internal/gitstate"
	"github.com/MiviaLabs/mivia-agentkit/internal/pathpolicy"
)

const stampRelPath = ".git/mivia-agent-quality-stamp.json"
const stampName = "mivia-agent-quality-stamp.json"

// Context configures a preflight run.
type Context struct {
	Repo              string
	ContractRows      []string
	FocusedVerifiers  []string // verifier IDs
	BroadVerifiers    []string // verifier IDs
	Verifiers         map[string]config.Verifier
	MutationProofs    []string
	NotRun            []string
	PipelinePreflight bool
}

// Run validates proof inputs and writes a fresh quality stamp.
func Run(ctx Context) (Stamp, error) {
	repo, err := gitstate.DetectRoot(defaultRepo(ctx.Repo))
	if err != nil {
		return Stamp{}, err
	}
	if ctx.Verifiers == nil {
		quality, err := loadQuality(repo)
		if err != nil {
			return Stamp{}, err
		}
		ctx.Verifiers = quality.Verifiers
	}
	changed, err := gitstate.ChangedFiles(repo)
	if err != nil {
		return Stamp{}, err
	}
	changed, err = expandDirectoryEntries(repo, changed)
	if err != nil {
		return Stamp{}, err
	}
	head, err := gitstate.Head(repo)
	if err != nil {
		return Stamp{}, err
	}
	diff, err := gitstate.DiffHash(repo, changed)
	if err != nil {
		return Stamp{}, err
	}
	if err := validateProofs(changed, ctx); err != nil {
		return Stamp{}, err
	}
	stamp := NewStamp(head, diff, changed)
	indexTree, err := gitstate.IndexTree(repo)
	if err != nil {
		return Stamp{}, err
	}
	stamp.Subject = Subject{BaseHead: head, IndexTree: indexTree}
	stamp.ContractRows = sortedCopy(ctx.ContractRows)
	stamp.FocusedVerifiers = sortedCopy(ctx.FocusedVerifiers)
	stamp.BroadVerifiers = sortedCopy(ctx.BroadVerifiers)
	stamp.MutationProofs = sortedCopy(ctx.MutationProofs)
	stamp.NotRun = sortedCopy(ctx.NotRun)
	stamp.VerifierEvidence, err = runVerifiers(repo, ctx.Verifiers, append(append([]string{}, ctx.FocusedVerifiers...), ctx.BroadVerifiers...), subjectHash(stamp.Subject))
	if err != nil {
		return Stamp{}, err
	}
	data, err := stamp.Marshal()
	if err != nil {
		return Stamp{}, err
	}
	stampRoot, _, err := stampLocation(repo)
	if err != nil {
		return Stamp{}, err
	}
	if err := pathpolicy.WriteFile(stampRoot, stampName, data, 0o600); err != nil {
		return Stamp{}, fmt.Errorf("write quality stamp: %w", err)
	}
	return stamp, nil
}

func runVerifiers(repo string, definitions map[string]config.Verifier, ids []string, subject string) ([]VerifierEvidence, error) {
	if len(ids) == 0 {
		return nil, fmt.Errorf("at least one verifier must run")
	}
	evidence := make([]VerifierEvidence, 0, len(ids))
	seen := map[string]struct{}{}
	for _, id := range ids {
		definition, ok := definitions[id]
		if !ok || len(definition.Command) == 0 || definition.Command[0] == "" {
			return nil, fmt.Errorf("trusted verifier %q is not declared", id)
		}
		if _, duplicate := seen[id]; duplicate {
			return nil, fmt.Errorf("duplicate verifier %q", id)
		}
		seen[id] = struct{}{}
		started := time.Now().UTC()
		cmd := exec.Command(definition.Command[0], definition.Command[1:]...)
		cmd.Dir = repo
		err := cmd.Run()
		finished := time.Now().UTC()
		result := VerifierEvidence{SchemaVersion: 1, CommandID: id, DefinitionHash: verifierHash(definition), SubjectHash: subject, StartedAt: started.Format(time.RFC3339), FinishedAt: finished.Format(time.RFC3339), ToolVersion: "local", Source: "local"}
		if err != nil {
			if exit, ok := err.(*exec.ExitError); ok {
				result.ExitCode = exit.ExitCode()
				evidence = append(evidence, result)
				return nil, fmt.Errorf("verifier %s exited %d", result.CommandID, result.ExitCode)
			}
			return nil, fmt.Errorf("run verifier %s: %w", result.CommandID, err)
		}
		evidence = append(evidence, result)
	}
	return evidence, nil
}

func subjectHash(subject Subject) string {
	sum := sha256.Sum256([]byte(subject.BaseHead + "\x00" + subject.IndexTree))
	return hex.EncodeToString(sum[:])
}

func verifierHash(verifier config.Verifier) string {
	sum := sha256.Sum256([]byte(strings.Join(verifier.Command, "\x00")))
	return hex.EncodeToString(sum[:])
}

func validateProofs(changed []string, ctx Context) error {
	var missing []string
	for _, reason := range ctx.NotRun {
		if strings.TrimSpace(reason) == "" {
			return fmt.Errorf("not-run reason must be non-empty")
		}
	}
	if len(ctx.NotRun) > 0 {
		return fmt.Errorf("protected stamps cannot contain not-run evidence")
	}
	if Classify(changed, ContractMatrix{RequireContractRowsFor: ctx.ContractRows}) == High {
		if len(ctx.ContractRows) == 0 {
			missing = append(missing, "contract row")
		}
		if len(ctx.FocusedVerifiers) == 0 {
			missing = append(missing, "focused verifier")
		}
		if len(ctx.MutationProofs) == 0 {
			missing = append(missing, "mutation proof")
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("missing required preflight proof: %s", strings.Join(missing, ", "))
	}
	return nil
}

func expandDirectoryEntries(repo string, files []string) ([]string, error) {
	expanded := make([]string, 0, len(files))
	for _, file := range files {
		path := filepath.Join(repo, filepath.FromSlash(file))
		info, err := os.Stat(path)
		if err != nil {
			expanded = append(expanded, filepath.ToSlash(file))
			continue
		}
		if !info.IsDir() {
			expanded = append(expanded, filepath.ToSlash(file))
			continue
		}
		err = filepath.WalkDir(path, func(child string, entry os.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if entry.IsDir() {
				return nil
			}
			rel, err := filepath.Rel(repo, child)
			if err != nil {
				return err
			}
			expanded = append(expanded, filepath.ToSlash(rel))
			return nil
		})
		if err != nil {
			return nil, fmt.Errorf("expand directory %s: %w", file, err)
		}
	}
	sort.Strings(expanded)
	return expanded, nil
}

func stampLocation(repo string) (string, string, error) {
	dotGit := filepath.Join(repo, ".git")
	info, err := os.Lstat(dotGit)
	if err != nil {
		return "", "", fmt.Errorf("inspect .git: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return "", "", fmt.Errorf(".git must not be a symlink")
	}
	cmd := exec.Command("git", "-C", repo, "rev-parse", "--git-path", stampName)
	out, err := cmd.Output()
	if err != nil {
		return "", "", fmt.Errorf("resolve Git stamp path: %w", err)
	}
	path := strings.TrimSpace(string(out))
	if !filepath.IsAbs(path) {
		path = filepath.Join(repo, path)
	}
	path, err = filepath.Abs(path)
	if err != nil {
		return "", "", fmt.Errorf("absolute Git stamp path: %w", err)
	}
	if filepath.Base(path) != stampName {
		return "", "", fmt.Errorf("invalid Git stamp path %q", path)
	}
	if info.IsDir() {
		expected := filepath.Join(repo, stampRelPath)
		if filepath.Clean(path) != filepath.Clean(expected) {
			return "", "", fmt.Errorf("quality stamp must stay under .git")
		}
	}
	return filepath.Dir(path), path, nil
}

func defaultRepo(repo string) string {
	if repo != "" {
		return repo
	}
	return "."
}
