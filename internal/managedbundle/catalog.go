package managedbundle

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

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
	return writeAtomicJSON(manifestPath, m)
}

// WriteJournal writes an active transaction journal under homeDir/.gentle-ai/managed/v1/transactions/<tx>/journal.json.
func WriteJournal(homeDir string, j MigrationJournal) error {
	txID := strings.TrimSpace(j.TransactionID)
	if txID == "" || strings.Contains(txID, "/") || strings.Contains(txID, "\\") || txID == "." || txID == ".." {
		return errors.New("invalid transaction ID: path traversal or unsafe characters detected")
	}

	dir := filepath.Join(homeDir, ManagedDirName, ManagedSubDir, JournalDirName, txID)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create transaction dir: %w", err)
	}

	journalPath := filepath.Join(dir, JournalFileName)
	return writeAtomicJSON(journalPath, j)
}

func writeAtomicJSON(targetPath string, v any) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal json: %w", err)
	}

	dir := filepath.Dir(targetPath)
	tmpFile, err := os.CreateTemp(dir, "atomic-*.tmp")
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	tmpName := tmpFile.Name()
	defer func() {
		_ = os.Remove(tmpName)
	}()

	if _, err := tmpFile.Write(data); err != nil {
		_ = tmpFile.Close()
		return fmt.Errorf("write temp file: %w", err)
	}
	if err := tmpFile.Sync(); err != nil {
		_ = tmpFile.Close()
		return fmt.Errorf("sync temp file: %w", err)
	}
	if err := tmpFile.Close(); err != nil {
		return fmt.Errorf("close temp file: %w", err)
	}

	if err := os.Chmod(tmpName, 0644); err != nil {
		return fmt.Errorf("chmod temp file: %w", err)
	}

	if err := os.Rename(tmpName, targetPath); err != nil {
		return fmt.Errorf("atomic rename to target: %w", err)
	}

	// Sync parent directory
	if parentDir, err := os.Open(dir); err == nil {
		_ = parentDir.Sync()
		_ = parentDir.Close()
	}

	return nil
}
