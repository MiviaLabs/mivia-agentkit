// Package doctor validates installed mivia-agent control surfaces.
// Plan: WS3. PRD: FR-2.1, FR-5.4, FR-6.1, FR-6.2.
package doctor

import (
	"bytes"
	"fmt"

	"github.com/MiviaLabs/mivia-agentkit/internal/config"
	"github.com/MiviaLabs/mivia-agentkit/internal/report"
	"gopkg.in/yaml.v3"
)

type rawManifest struct {
	Profile    string                          `yaml:"profile"`
	Adapters   map[string]config.AdapterConfig `yaml:"adapters"`
	Loops      map[string]rawLoop              `yaml:"loops"`
	Governance config.Governance               `yaml:"governance"`
}

type rawLoop struct {
	Bound         string        `yaml:"bound"`
	MaxIterations int           `yaml:"max_iterations"`
	Steps         []config.Step `yaml:"steps"`
	ExitWhen      string        `yaml:"exit_when"`
}

func raw(ctx *Context) (rawManifest, error) {
	var parsed rawManifest
	if len(ctx.manifest.raw) == 0 {
		return parsed, ctx.manifest.err
	}
	dec := yaml.NewDecoder(bytes.NewReader(ctx.manifest.raw))
	dec.KnownFields(false)
	return parsed, dec.Decode(&parsed)
}

func checkLoopsBound(ctx *Context) []report.Finding {
	parsed, err := raw(ctx)
	if err != nil {
		return nil
	}
	var findings []report.Finding
	for name, loop := range parsed.Loops {
		if loop.Bound == "" || loop.Bound == "budget" || loop.MaxIterations <= 0 {
			findings = append(findings, finding(report.SeverityError, "loop.unbounded", "mivia-agent.yaml", fmt.Sprintf("loop %q must use iteration bound with positive max_iterations", name)))
		}
	}
	return findings
}

func checkLoopsKnownAdapters(ctx *Context) []report.Finding {
	parsed, err := raw(ctx)
	if err != nil {
		return nil
	}
	enabled := map[string]config.AdapterRole{}
	for name, adapter := range parsed.Adapters {
		if adapter.Enabled {
			enabled[name] = adapter.Role
		}
	}
	var findings []report.Finding
	for loopName, loop := range parsed.Loops {
		for _, step := range loop.Steps {
			if step.Producer != "" {
				if role, ok := enabled[step.Producer]; !ok || role != config.AdapterRoleOrchestrable {
					findings = append(findings, finding(report.SeverityError, "loop.unknown_adapter", "mivia-agent.yaml", fmt.Sprintf("loop %q step %q references unavailable producer %q", loopName, step.ID, step.Producer)))
				}
			}
			for _, reviewer := range step.Reviewers {
				if role, ok := enabled[reviewer]; !ok || role != config.AdapterRoleOrchestrable {
					findings = append(findings, finding(report.SeverityError, "loop.unknown_adapter", "mivia-agent.yaml", fmt.Sprintf("loop %q step %q references unavailable reviewer %q", loopName, step.ID, reviewer)))
				}
			}
		}
	}
	return findings
}

func checkConsensusSatisfiable(ctx *Context) []report.Finding {
	parsed, err := raw(ctx)
	if err != nil {
		return nil
	}
	enabledOrchestrable := 0
	for _, adapter := range parsed.Adapters {
		if adapter.Enabled && adapter.Role == config.AdapterRoleOrchestrable {
			enabledOrchestrable++
		}
	}
	var findings []report.Finding
	for loopName, loop := range parsed.Loops {
		for _, step := range loop.Steps {
			minReviewers := step.Consensus.MinReviewers
			if minReviewers == 0 {
				minReviewers = len(step.Reviewers)
			}
			if len(step.Reviewers) > 0 && minReviewers > enabledOrchestrable {
				findings = append(findings, finding(report.SeverityError, "consensus.unsatisfiable", "mivia-agent.yaml", fmt.Sprintf("loop %q step %q requires %d reviewers but only %d orchestrable adapters are enabled", loopName, step.ID, minReviewers, enabledOrchestrable)))
			}
		}
	}
	return findings
}

func checkGovernanceKnown(ctx *Context) []report.Finding {
	parsed, err := raw(ctx)
	if err != nil {
		return nil
	}
	switch parsed.Governance.Provider {
	case "", "noop", "agt":
		return nil
	default:
		return []report.Finding{finding(report.SeverityError, "governance.provider_unknown", "mivia-agent.yaml", fmt.Sprintf("unknown governance provider %q", parsed.Governance.Provider))}
	}
}
