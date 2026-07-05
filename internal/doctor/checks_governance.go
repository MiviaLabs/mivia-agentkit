// Package doctor validates installed mivia-agent control surfaces.
// Plan: WS12. PRD: FR-2.2, FR-7.2.
package doctor

import (
	"errors"

	"github.com/MiviaLabs/mivia-agentkit/internal/policy"
	"github.com/MiviaLabs/mivia-agentkit/internal/report"
)

func checkGovernanceProviderCompilable(ctx *Context) []report.Finding {
	parsed, err := raw(ctx)
	if err != nil {
		return nil
	}
	if parsed.Governance.Provider != "agt" {
		return nil
	}
	_, err = policy.New("agt", parsed.Governance.AuditLog)
	if err == nil {
		return nil
	}
	severity := report.SeverityWarn
	if parsed.Profile == "strict" || ctx.Strict {
		severity = report.SeverityError
	}
	code := "governance.provider_unavailable"
	if errors.Is(err, policy.ErrAGTNotCompiled) {
		code = "governance.agt_required_unavailable"
	}
	return []report.Finding{finding(severity, code, "mivia-agent.yaml", err.Error())}
}
