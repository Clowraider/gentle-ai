package screens

import (
	"fmt"
	"strings"

	"github.com/gentleman-programming/gentle-ai/v2/internal/agentbuilder"
	"github.com/gentleman-programming/gentle-ai/v2/internal/tui/styles"
)

// RenderCustomAgents renders the Custom Agents list and management screen.
// It displays existing custom agents and provides actions to create, delete, or return.
func RenderCustomAgents(agents []agentbuilder.RegistryEntry, cursor int, err error, hasEngines bool) string {
	var b strings.Builder

	b.WriteString(styles.TitleStyle.Render("Manage Custom Agents"))
	b.WriteString("\n\n")
	b.WriteString(styles.SubtextStyle.Render("Create or remove custom agents installed across your configured AI tools."))
	b.WriteString("\n\n")

	if err != nil {
		b.WriteString(styles.WarningStyle.Render("Error: " + err.Error()))
		b.WriteString("\n\n")
	}

	if len(agents) == 0 {
		emptyMsg := "No custom agents created yet. Use 'Create new agent' to build one."
		if !hasEngines {
			emptyMsg = "No custom agents created yet. Install an agent-builder engine to create one."
		}
		b.WriteString(styles.SubtextStyle.Render(emptyMsg))
		b.WriteString("\n\n")
	}

	options := make([]string, 0, len(agents)+2)
	for _, a := range agents {
		label := fmt.Sprintf("• %s", a.Name)
		if a.Title != "" {
			label = fmt.Sprintf("• %s ─── %s", a.Name, a.Title)
		}
		options = append(options, label)
	}

	createLabel := "Create new agent"
	if !hasEngines {
		createLabel = "Create new agent (no engine available)"
	}
	options = append(options, createLabel)
	options = append(options, "Back")

	b.WriteString(renderOptions(options, cursor))
	b.WriteString("\n")
	b.WriteString(styles.HelpStyle.Render("j/k: navigate • enter: select/create • d: delete • esc: back"))

	return styles.FrameStyle.Render(b.String())
}

// CustomAgentsOptionCount returns the number of selectable options on the screen.
func CustomAgentsOptionCount(agents []agentbuilder.RegistryEntry) int {
	return len(agents) + 2
}

// RenderCustomAgentDelete renders the deletion confirmation screen for a custom agent.
func RenderCustomAgentDelete(agentName string, cursor int) string {
	var b strings.Builder

	b.WriteString(styles.TitleStyle.Render("Delete Custom Agent"))
	b.WriteString("\n\n")

	b.WriteString(styles.WarningStyle.Render(fmt.Sprintf("Are you sure you want to delete custom agent %q?", agentName)))
	b.WriteString("\n\n")

	b.WriteString(styles.SubtextStyle.Render("This will remove the agent from the registry and delete its SKILL.md from all installed agent skill directories."))
	b.WriteString("\n\n")

	b.WriteString(styles.WarningStyle.Render("This action cannot be undone."))
	b.WriteString("\n\n")

	b.WriteString(renderOptions([]string{"Delete Agent", "Cancel"}, cursor))
	b.WriteString("\n")
	b.WriteString(styles.HelpStyle.Render("j/k: navigate • enter: confirm • esc: back"))

	return styles.FrameStyle.Render(b.String())
}

// CustomAgentDeleteOptionCount returns the number of options on confirmation screen (2).
func CustomAgentDeleteOptionCount() int {
	return 2
}
