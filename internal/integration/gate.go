// Package integration gates real runtime coverage on explicit opt-in and local prerequisites.
// Plan: WS14. PRD: §3, §9, §14.
package integration

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// RealCLITestsEnv enables opt-in local CLI integration coverage.
const RealCLITestsEnv = "MIVIA_AGENT_REAL_CLI_TESTS"

// ToolStatus reports one integration prerequisite.
type ToolStatus struct {
	Name      string
	Available bool
	Reason    string
}

// Gate centralizes opt-in and prerequisite checks for real runtime coverage.
type Gate struct {
	Enabled  bool
	OptInEnv string
}

// DefaultGate returns the repo-default local CLI gate.
func DefaultGate() Gate {
	return Gate{
		Enabled:  strings.TrimSpace(os.Getenv(RealCLITestsEnv)) == "1",
		OptInEnv: RealCLITestsEnv,
	}
}

// RequireBinary reports whether a local executable is available.
func (g Gate) RequireBinary(name string) ToolStatus {
	if _, err := exec.LookPath(name); err != nil {
		return ToolStatus{
			Name:      name,
			Available: false,
			Reason:    fmt.Sprintf("missing binary %q", name),
		}
	}
	return ToolStatus{Name: name, Available: true}
}

// RequireEnv reports whether a required environment variable is set.
func (g Gate) RequireEnv(name string) ToolStatus {
	if strings.TrimSpace(os.Getenv(name)) == "" {
		return ToolStatus{
			Name:      name,
			Available: false,
			Reason:    fmt.Sprintf("missing environment variable %q", name),
		}
	}
	return ToolStatus{Name: name, Available: true}
}

// Allow reports whether real integration coverage should run.
func (g Gate) Allow(statuses ...ToolStatus) (bool, string) {
	if !g.Enabled {
		optInEnv := g.OptInEnv
		if optInEnv == "" {
			optInEnv = RealCLITestsEnv
		}
		return false, fmt.Sprintf("set %s=1 to enable real CLI integration coverage", optInEnv)
	}
	for _, status := range statuses {
		if !status.Available {
			return false, status.Reason
		}
	}
	return true, ""
}
