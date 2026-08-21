package copilotcli

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/model"
	"github.com/gentleman-programming/gentle-ai/v2/internal/system"
)

func TestCopilotCLIAdapter_IdentityAndCapabilities(t *testing.T) {
	a := NewAdapter()
	if a.Agent() != model.AgentGitHubCopilotCLI || a.Tier() != model.TierFull {
		t.Errorf("Agent/Tier unexpected: %v / %v", a.Agent(), a.Tier())
	}
	if a.SupportsOutputStyles() || a.SupportsSlashCommands() || a.SupportsSubAgents() {
		t.Error("unexpected capability enabled")
	}
	if !a.SupportsSkills() || !a.SupportsSystemPrompt() || !a.SupportsMCP() {
		t.Error("expected capability disabled")
	}
	if a.OutputStyleDir("") != "" || a.CommandsDir("") != "" || a.SubAgentsDir("") != "" || a.EmbeddedSubAgentsDir() != "" {
		t.Error("dirs should be empty")
	}
}

func TestCopilotCLIAdapter_Detect(t *testing.T) {
	t.Run("found", func(t *testing.T) {
		a := &Adapter{
			lookPath: func(string) (string, error) { return "/bin/copilot", nil },
			statPath: func(string) statResult { return statResult{isDir: true} },
		}
		ins, bin, cfg, cfgFound, err := a.Detect(context.Background(), "/home/u")
		if err != nil || !ins || bin != "/bin/copilot" || cfg != "/home/u/.copilot" || !cfgFound {
			t.Errorf("got (%v, %q, %q, %v, %v)", ins, bin, cfg, cfgFound, err)
		}
	})

	t.Run("missing", func(t *testing.T) {
		a := &Adapter{
			lookPath: func(string) (string, error) { return "", exec.ErrNotFound },
			statPath: func(string) statResult { return statResult{err: os.ErrNotExist} },
		}
		ins, bin, cfg, cfgFound, err := a.Detect(context.Background(), "/home/u")
		if err != nil || ins || bin != "" || cfg != "/home/u/.copilot" || cfgFound {
			t.Errorf("got (%v, %q, %q, %v, %v)", ins, bin, cfg, cfgFound, err)
		}
	})

	t.Run("err", func(t *testing.T) {
		boom := errors.New("boom")
		a := &Adapter{
			lookPath: func(string) (string, error) { return "/bin/copilot", nil },
			statPath: func(string) statResult { return statResult{err: boom} },
		}
		if _, _, _, _, err := a.Detect(context.Background(), "/home/u"); !errors.Is(err, boom) {
			t.Errorf("got %v, want %v", err, boom)
		}
	})
}

func TestCopilotCLIAdapter_PathsAndInstall(t *testing.T) {
	a := NewAdapter()
	home := "/home/u"

	if a.GlobalConfigDir(home) != filepath.Join(home, ".copilot") ||
		a.SystemPromptDir(home) != filepath.Join(home, ".copilot") ||
		a.SystemPromptFile(home) != filepath.Join(home, ".copilot", "copilot-instructions.md") ||
		a.SkillsDir(home) != filepath.Join(home, ".copilot", "skills") ||
		a.SettingsPath(home) != filepath.Join(home, ".copilot", "settings.json") ||
		a.MCPConfigPath(home, "") != filepath.Join(home, ".copilot", "mcp-config.json") ||
		a.SystemPromptStrategy() != model.StrategyFileReplace ||
		a.MCPStrategy() != model.StrategyMCPConfigFile {
		t.Error("unexpected path or strategy")
	}

	cmd, err := a.InstallCommand(system.PlatformProfile{OS: "linux", NpmWritable: true})
	if err != nil || len(cmd) == 0 || cmd[0][0] != "npm" || cmd[0][len(cmd[0])-1] != "@github/copilot@latest" {
		t.Errorf("InstallCommand(writable) = %v, %v", cmd, err)
	}

	cmd, err = a.InstallCommand(system.PlatformProfile{OS: "linux", NpmWritable: false})
	if err != nil || len(cmd) == 0 || cmd[0][0] != "sudo" || cmd[0][len(cmd[0])-1] != "@github/copilot@latest" {
		t.Errorf("InstallCommand(not writable) = %v, %v", cmd, err)
	}
}
