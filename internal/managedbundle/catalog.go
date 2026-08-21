package managedbundle

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/gentleman-programming/gentle-ai/v2/internal/assets"
	"github.com/gentleman-programming/gentle-ai/v2/internal/model"
)

const (
	DefaultArchiveSkillResourceID = "opencode.skill/sdd-archive@1"
	DefaultArchiveSkillRelPath    = ".config/opencode/skills/sdd-archive/SKILL.md"
)

// DefaultCatalog builds the default catalog for the active version of gentle-ai.
// For Slice 1, it catalogs the full-file sdd-archive skill resource.
func DefaultCatalog(version, vcsRevision string) ResourceCatalog {
	return ResourceCatalog{
		Version:     version,
		VCSRevision: vcsRevision,
		Resources: []ResourceDescriptor{
			{
				ResourceID:    DefaultArchiveSkillResourceID,
				SchemaVersion: 1,
				ComponentID:   model.ComponentSkills,
				AgentID:       model.AgentOpenCode,
				CanonicalPath: DefaultArchiveSkillRelPath,
				Ownership:     OwnershipDescriptor{Kind: OwnershipFullFile},
				RenderDesired: func() (DesiredExtent, error) {
					content, err := assets.FS.ReadFile("skills/sdd-archive/SKILL.md")
					if err != nil {
						return DesiredExtent{}, fmt.Errorf("read embedded sdd-archive skill: %w", err)
					}
					sum := sha256.Sum256(content)
					return DesiredExtent{
						Content: content,
						SHA256:  "sha256:" + hex.EncodeToString(sum[:]),
						Mode:    0644,
					}, nil
				},
			},
		},
	}
}

// WriteManifest writes the manifest atomically under homeDir/.gentle-ai/managed/v1/manifest.json.
func WriteManifest(homeDir string, m ManagedBundleManifest) error {
	dir := filepath.Join(homeDir, ManagedDirName, ManagedSubDir)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create managed dir: %w", err)
	}

	manifestPath := filepath.Join(dir, ManifestFileName)
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal manifest: %w", err)
	}

	tmpPath := manifestPath + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0644); err != nil {
		return fmt.Errorf("write temp manifest: %w", err)
	}
	if err := os.Rename(tmpPath, manifestPath); err != nil {
		return fmt.Errorf("commit manifest: %w", err)
	}
	return nil
}

// WriteJournal writes an active transaction journal under homeDir/.gentle-ai/managed/v1/transactions/<tx>/journal.json.
func WriteJournal(homeDir string, j MigrationJournal) error {
	dir := filepath.Join(homeDir, ManagedDirName, ManagedSubDir, JournalDirName, j.TransactionID)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create transaction dir: %w", err)
	}

	journalPath := filepath.Join(dir, JournalFileName)
	data, err := json.MarshalIndent(j, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal journal: %w", err)
	}

	tmpPath := journalPath + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0644); err != nil {
		return fmt.Errorf("write temp journal: %w", err)
	}
	if err := os.Rename(tmpPath, journalPath); err != nil {
		return fmt.Errorf("commit journal: %w", err)
	}
	return nil
}
