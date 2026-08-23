package cli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/managedbundle"
	"github.com/gentleman-programming/gentle-ai/v2/internal/model"
)

func TestProductionManagedBundleTransactionWritesDoctorEvidence(t *testing.T) {
	home := t.TempDir()
	transaction, err := prepareManagedBundleTransaction(home, ScopeGlobal, []model.AgentID{model.AgentOpenCode})
	if err != nil {
		t.Fatal(err)
	}
	if transaction == nil {
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
	if err := transaction.VerifyAndCommit(AppVersion, ""); err != nil {
		t.Fatal(err)
	}
	result := checkInstalledAssetVersion(home)
	if result.Status != CheckStatusPass {
		t.Fatalf("doctor result = %#v", result)
	}
}

func TestManagedBundleTransactionOnlyOwnsGlobalOpenCode(t *testing.T) {
	for _, tt := range []struct {
		name   string
		scope  InstallScope
		agents []model.AgentID
	}{
		{name: "workspace", scope: ScopeWorkspace, agents: []model.AgentID{model.AgentOpenCode}},
		{name: "other agent", scope: ScopeGlobal, agents: []model.AgentID{model.AgentClaudeCode}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			transaction, err := prepareManagedBundleTransaction(t.TempDir(), tt.scope, tt.agents)
			if err != nil || transaction != nil {
				t.Fatalf("transaction = %v, err = %v", transaction, err)
			}
		})
	}
}
