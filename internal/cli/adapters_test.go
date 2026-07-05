// Package cli implements the mivia-agent command surface.
// Plan: WS13. PRD: FR-3.2.
package cli

import (
	"context"
	"encoding/json"
	"sync"
	"testing"

	"github.com/MiviaLabs/mivia-agentkit/internal/adapter"
	"github.com/MiviaLabs/mivia-agentkit/internal/config"
)

func TestAdaptersReportsHeadlessCapability(t *testing.T) {
	withRuntimeAdapters(t, fakeCLIAdapter{name: "codex", headless: true})
	manifest := config.Defaults()
	manifest.Adapters = map[string]config.AdapterConfig{"codex": {Enabled: true, Role: config.AdapterRoleOrchestrable}}
	statuses, err := detectAdapters(context.Background(), manifest)
	if err != nil {
		t.Fatalf("detectAdapters() error = %v", err)
	}
	if len(statuses) != 1 || !statuses[0].Installed || !statuses[0].Headless || !statuses[0].ApprovedForRun {
		t.Fatalf("statuses = %#v, want installed headless approved", statuses)
	}
}

func TestAdaptersExitsNonZeroWhenOrchestrableAdapterNotHeadless(t *testing.T) {
	withRuntimeAdapters(t, fakeCLIAdapter{name: "codex", headless: false})
	manifest := config.Defaults()
	manifest.Adapters = map[string]config.AdapterConfig{"codex": {Enabled: true, Role: config.AdapterRoleOrchestrable}}
	statuses, err := detectAdapters(context.Background(), manifest)
	if err == nil {
		t.Fatalf("detectAdapters() error = nil statuses=%#v, want non-headless orchestrable rejection", statuses)
	}
	if statuses[0].ApprovedForRun {
		t.Fatalf("ApprovedForRun = true, want false when headless is false")
	}
}

func TestAdaptersJSONShape(t *testing.T) {
	status := []adapterStatus{{Name: "codex", Installed: true, Version: "1.0", Headless: true, Role: "orchestrable", ApprovedForRun: true}}
	data, err := json.Marshal(status)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	if !containsAll(string(data), "approved_for_run", "headless", "version") {
		t.Fatalf("json = %s, want adapter status fields", data)
	}
}

func TestAdaptersReportsManifestGuidanceWithoutRuntimeAdapter(t *testing.T) {
	withRuntimeAdapters(t, fakeCLIAdapter{name: "codex", headless: true}, fakeCLIAdapter{name: "claude", headless: true})
	statuses, err := detectAdapters(context.Background(), config.Defaults())
	if err != nil {
		t.Fatalf("detectAdapters() error = %v", err)
	}
	var found bool
	for _, status := range statuses {
		if status.Name == "copilot" {
			found = true
			if status.Role != "guidance" || status.ApprovedForRun || status.Installed {
				t.Fatalf("copilot status = %#v, want guidance not approved for run", status)
			}
		}
	}
	if !found {
		t.Fatalf("statuses = %#v, want copilot guidance entry", statuses)
	}
}

type fakeCLIAdapter struct {
	name     string
	headless bool
	run      adapter.Result
	verdict  adapter.Verdict
	calls    *int
	prompts  *[]string
	runReqs  *[]adapter.Request
	reviews  *[]adapter.Request
}

var fakeAdapterMu sync.Mutex

func (f fakeCLIAdapter) Name() string { return f.name }
func (f fakeCLIAdapter) Role() adapter.Role {
	return adapter.RoleOrchestrable
}
func (f fakeCLIAdapter) Detect(context.Context) (adapter.Detection, error) {
	return adapter.Detection{Name: f.name, Version: "fake-1", HeadlessCapable: f.headless}, nil
}
func (f fakeCLIAdapter) Run(_ context.Context, req adapter.Request) (adapter.Result, error) {
	fakeAdapterMu.Lock()
	defer fakeAdapterMu.Unlock()
	if f.calls != nil {
		*f.calls++
	}
	if f.prompts != nil {
		*f.prompts = append(*f.prompts, req.Prompt)
	}
	if f.runReqs != nil {
		*f.runReqs = append(*f.runReqs, req)
	}
	return f.run, nil
}
func (f fakeCLIAdapter) Review(_ context.Context, req adapter.Request) (adapter.Verdict, error) {
	fakeAdapterMu.Lock()
	defer fakeAdapterMu.Unlock()
	if f.calls != nil {
		*f.calls++
	}
	if f.prompts != nil {
		*f.prompts = append(*f.prompts, req.Prompt)
	}
	if f.reviews != nil {
		*f.reviews = append(*f.reviews, req)
	}
	return f.verdict, nil
}

func withRuntimeAdapters(t *testing.T, adapters ...adapter.Adapter) {
	t.Helper()
	old := runtimeAdapters
	runtimeAdapters = func() []adapter.Adapter { return adapters }
	t.Cleanup(func() { runtimeAdapters = old })
}
