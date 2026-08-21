package screens

import (
	"errors"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/agentbuilder"
)

func TestRenderCustomAgents_EmptyAndPopulated(t *testing.T) {
	out := RenderCustomAgents(nil, 0, nil)
	if !strings.Contains(out, "Manage Custom Agents") || !strings.Contains(out, "No custom agents created yet") {
		t.Errorf("expected empty message, got: %s", out)
	}

	agents := []agentbuilder.RegistryEntry{
		{Name: "agent-one", Title: "Agent One"},
		{Name: "agent-two", Title: "Agent Two"},
	}
	out = RenderCustomAgents(agents, 0, errors.New("sample error"))
	if !strings.Contains(out, "agent-one ─── Agent One") || !strings.Contains(out, "Error: sample error") {
		t.Errorf("expected agent listed and error displayed, got: %s", out)
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
