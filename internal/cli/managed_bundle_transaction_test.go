package cli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/managedbundle"
	"github.com/gentleman-programming/gentle-ai/v2/internal/model"
	"github.com/gentleman-programming/gentle-ai/v2/internal/pipeline"
	"github.com/gentleman-programming/gentle-ai/v2/internal/planner"
)

func TestProductionManagedBundleTransactionWritesDoctorEvidence(t *testing.T) {
	home := t.TempDir()
	state := &runtimeState{}
	if err := (managedBundlePrepareStep{homeDir: home, state: state}).Run(); err != nil {
		t.Fatal(err)
	}
	if state.managedBundle == nil {
		t.Fatal("global OpenCode install did not prepare a transaction")
	}
	catalog, err := managedbundle.Catalog()
	if err != nil {
		t.Fatal(err)
	}
	descriptor := catalog[0]
	target := managedbundle.ResolveTarget(home, descriptor)
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, descriptor.Content, os.FileMode(descriptor.Mode)); err != nil {
		t.Fatal(err)
	}
	if err := state.managedBundle.VerifyAndCommit(AppVersion, ""); err != nil {
		t.Fatal(err)
	}
	result := checkInstalledAssetVersion(home)
	if result.Status != CheckStatusPass {
		t.Fatalf("doctor result = %#v", result)
	}
}

func TestManagedBundleTransactionFollowsBackupInProductionPlans(t *testing.T) {
	install := (&installRuntime{homeDir: t.TempDir(), scope: ScopeGlobal, resolved: planner.ResolvedPlan{Agents: []model.AgentID{model.AgentOpenCode}, OrderedComponents: []model.ComponentID{model.ComponentSDD}}, state: &runtimeState{}}).stagePlan()
	if got := stepIDs(install.Prepare); len(got) != 3 || got[1] != "prepare:backup-snapshot" || got[2] != "prepare:managed-bundle" {
		t.Fatalf("install prepare order = %v", got)
	}
	sync := (&syncRuntime{homeDir: t.TempDir(), agentIDs: []model.AgentID{model.AgentOpenCode}, selection: model.Selection{Agents: []model.AgentID{model.AgentOpenCode}, Components: []model.ComponentID{model.ComponentSDD}}, state: &runtimeState{}}).stagePlan()
	if got := stepIDs(sync.Prepare); len(got) != 2 || got[0] != "prepare:backup-snapshot" || got[1] != "prepare:managed-bundle" {
		t.Fatalf("sync prepare order = %v", got)
	}
}

func stepIDs(steps []pipeline.Step) []string {
	ids := make([]string, 0, len(steps))
	for _, step := range steps {
		ids = append(ids, step.ID())
	}
	return ids
}
