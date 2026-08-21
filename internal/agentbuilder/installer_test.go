package agentbuilder

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/model"
)

func makeAgent(name, content string) *GeneratedAgent {
	return &GeneratedAgent{
		Name:    name,
		Title:   "Test Agent",
		Content: content,
	}
}

func TestInstallAndUninstall_HappyPath(t *testing.T) {
	dir1, dir2 := t.TempDir(), t.TempDir()
	agent := makeAgent("my-agent", "# My Agent\nContent")
	adapters := []AdapterInfo{
		{AgentID: model.AgentClaudeCode, SkillsDir: dir1},
		{AgentID: model.AgentOpenCode, SkillsDir: dir2},
	}

	results, err := Install(agent, adapters, "")
	if err != nil || len(results) != 2 {
		t.Fatalf("Install failed: %v", err)
	}

	f1 := filepath.Join(dir1, "my-agent", "SKILL.md")
	f2 := filepath.Join(dir2, "my-agent", "SKILL.md")
	if _, err := os.Stat(f1); err != nil {
		t.Fatalf("f1 missing: %v", err)
	}
	if _, err := os.Stat(f2); err != nil {
		t.Fatalf("f2 missing: %v", err)
	}

	removed, err := Uninstall("my-agent", adapters)
	if err != nil || len(removed) != 2 {
		t.Fatalf("Uninstall failed: %v, removed: %v", err, removed)
	}
	if _, err := os.Stat(f1); !os.IsNotExist(err) {
		t.Errorf("f1 still exists after uninstall")
	}
	if _, err := os.Stat(f2); !os.IsNotExist(err) {
		t.Errorf("f2 still exists after uninstall")
	}
}

func TestInstall_RollbackOnFailure(t *testing.T) {
	dir1, dir2 := t.TempDir(), t.TempDir()
	blocker := filepath.Join(dir2, "my-agent")
	_ = os.WriteFile(blocker, []byte("block"), 0444)
	_ = os.Chmod(blocker, 0444)

	agent := makeAgent("my-agent", "# Agent\n")
	adapters := []AdapterInfo{
		{AgentID: model.AgentClaudeCode, SkillsDir: dir1},
		{AgentID: model.AgentOpenCode, SkillsDir: dir2},
	}

	if _, err := Install(agent, adapters, ""); err == nil {
		t.Fatal("expected error on failure")
	}
	firstFile := filepath.Join(dir1, "my-agent", "SKILL.md")
	if _, err := os.Stat(firstFile); err == nil {
		t.Errorf("expected first file rollback, still exists: %s", firstFile)
	}
}

func TestInstall_EdgeCases(t *testing.T) {
	if _, err := Install(nil, []AdapterInfo{}, ""); err == nil {
		t.Fatal("expected error for nil agent")
	}
	if _, err := Uninstall("", []AdapterInfo{}); err == nil {
		t.Fatal("expected error for empty agent name")
	}
}
