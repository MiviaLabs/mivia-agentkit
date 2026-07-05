// Package orchestrator resolves and executes bounded agent loops.
// Plan: WS10. PRD: FR-4.1, FR-6.3.
package orchestrator

import (
	"fmt"

	"github.com/MiviaLabs/mivia-agentkit/internal/config"
)

// Node is one executable loop step plus dependencies.
type Node struct {
	Step      config.Step
	DependsOn []string
}

// Nodes is a resolved DAG.
type Nodes []Node

// Resolve converts a loop into ordered DAG nodes.
func Resolve(loop config.Loop) (Nodes, error) {
	nodes := make(Nodes, 0, len(loop.Steps))
	var previous string
	for _, step := range loop.Steps {
		n := Node{Step: step}
		if previous != "" {
			n.DependsOn = append(n.DependsOn, previous)
		}
		nodes = append(nodes, n)
		previous = step.ID
	}
	if err := nodes.Validate(); err != nil {
		return nil, err
	}
	return nodes, nil
}

// Validate rejects malformed or cyclic DAGs.
func (ns Nodes) Validate() error {
	seen := map[string]Node{}
	for _, n := range ns {
		if n.Step.ID == "" {
			return fmt.Errorf("step id is required")
		}
		if n.Step.Producer != "" && len(n.Step.Reviewers) > 0 {
			return fmt.Errorf("step %q cannot be both producer and review", n.Step.ID)
		}
		if _, ok := seen[n.Step.ID]; ok {
			return fmt.Errorf("duplicate step id %q", n.Step.ID)
		}
		seen[n.Step.ID] = n
	}
	for _, n := range ns {
		for _, dep := range n.DependsOn {
			if _, ok := seen[dep]; !ok {
				return fmt.Errorf("step %q depends on missing step %q", n.Step.ID, dep)
			}
		}
	}
	visiting := map[string]bool{}
	visited := map[string]bool{}
	var visit func(string) error
	visit = func(id string) error {
		if visiting[id] {
			return fmt.Errorf("cycle detected at step %q", id)
		}
		if visited[id] {
			return nil
		}
		visiting[id] = true
		for _, dep := range seen[id].DependsOn {
			if err := visit(dep); err != nil {
				return err
			}
		}
		visiting[id] = false
		visited[id] = true
		return nil
	}
	for id := range seen {
		if err := visit(id); err != nil {
			return err
		}
	}
	return nil
}
