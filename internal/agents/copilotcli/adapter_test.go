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

func TestCopilotCLIAdapter_Detect(t *testing.T) {
	t.Setenv("COPILOT_HOME", "")
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

func TestCopilotCLIAdapter_DetectUsesCopilotHome(t *testing.T) {
	root := filepath.Join(t.TempDir(), "custom-copilot")
	t.Setenv("COPILOT_HOME", root)
	a := &Adapter{
		lookPath: func(string) (string, error) { return "/bin/copilot", nil },
		statPath: func(path string) statResult {
			if path != root {
				t.Fatalf("stat path = %q, want %q", path, root)
			}
			return statResult{isDir: true}
		},
	}

	_, _, cfg, cfgFound, err := a.Detect(context.Background(), "/home/u")
	if err != nil || cfg != root || !cfgFound {
		t.Fatalf("Detect() config = %q, found = %v, err = %v", cfg, cfgFound, err)
	}
}

func TestCopilotCLIAdapter_PathsAndInstall(t *testing.T) {
	t.Setenv("COPILOT_HOME", "")
	a := NewAdapter()
	home := "/home/u"
	if a.GlobalConfigDir(home) != filepath.Join(home, ".copilot") || a.SystemPromptFile(home) != filepath.Join(home, ".copilot", "copilot-instructions.md") || a.SkillsDir(home) != filepath.Join(home, ".copilot", "skills") || a.SettingsPath(home) != filepath.Join(home, ".copilot", "settings.json") || a.MCPConfigPath(home, "") != filepath.Join(home, ".copilot", "mcp-config.json") || a.SystemPromptStrategy() != model.StrategyFileReplace || a.MCPStrategy() != model.StrategyMCPConfigFile {
		t.Fatal("unexpected path or strategy")
	}
	for _, tt := range []struct {
		profile system.PlatformProfile
		want    string
	}{{system.PlatformProfile{OS: "linux", NpmWritable: true}, "npm"}, {system.PlatformProfile{OS: "linux", NpmWritable: false}, "sudo"}} {
		cmd, err := a.InstallCommand(tt.profile)
		if err != nil || len(cmd) != 1 || cmd[0][0] != tt.want || cmd[0][len(cmd[0])-1] != "@github/copilot@latest" {
			t.Fatalf("InstallCommand(%+v) = %v, %v", tt.profile, cmd, err)
		}
	}
}
