package screens

import (
	"errors"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/agentbuilder"
)

func TestRenderCustomAgents_EmptyAndPopulated(t *testing.T) {
	out := RenderCustomAgents(nil, 0, nil, true)
	if !strings.Contains(out, "Manage Custom Agents") || !strings.Contains(out, "No custom agents created yet. Use 'Create new agent' to build one.") {
		t.Errorf("expected empty message with create instruction, got: %s", out)
	}

	outNoEngines := RenderCustomAgents(nil, 0, nil, false)
	if !strings.Contains(outNoEngines, "Install an agent-builder engine to create one.") {
		t.Errorf("expected engine installation instruction, got: %s", outNoEngines)
	}

	agents := []agentbuilder.RegistryEntry{
		{Name: "agent-one", Title: "Agent One"},
		{Name: "agent-two", Title: "Agent Two"},
	}
	out = RenderCustomAgents(agents, 0, errors.New("sample error"), false)
	if !strings.Contains(out, "agent-one ─── Agent One") || !strings.Contains(out, "Error: sample error") {
		t.Errorf("expected agent listed and error displayed, got: %s", out)
	}
	if !strings.Contains(out, "no engine available") {
		t.Errorf("expected disabled create label when hasEngines=false, got: %s", out)
	}
	if count := CustomAgentsOptionCount(agents); count != 4 {
		t.Errorf("CustomAgentsOptionCount = %d, want 4", count)
	}
}

func TestRenderCustomAgentDelete(t *testing.T) {
	out := RenderCustomAgentDelete("my-custom-agent", 0)
	if !strings.Contains(out, "Delete Custom Agent") || !strings.Contains(out, "my-custom-agent") {
		t.Errorf("expected title and agent name, got: %s", out)
	}
	if count := CustomAgentDeleteOptionCount(); count != 2 {
		t.Errorf("CustomAgentDeleteOptionCount = %d, want 2", count)
	}
}
