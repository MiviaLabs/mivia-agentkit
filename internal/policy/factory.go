// Package policy defines mivia-agent governance provider contracts.
// Plan: WS12. PRD: FR-2.2, FR-7.2.
package policy

import "fmt"

// New returns a governance provider by name.
func New(name, auditPath string) (Provider, error) {
	switch name {
	case "", "noop":
		return Noop{AuditPath: auditPath}, nil
	case "agt":
		return NewAGT(auditPath)
	default:
		return nil, fmt.Errorf("unknown governance provider %q", name)
	}
}
