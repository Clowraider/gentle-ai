package managedbundle

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"path/filepath"
	"sort"

	"github.com/gentleman-programming/gentle-ai/v2/internal/assets"
)

const (
	ManifestSchemaV1  = "gentle-ai.managed-bundle/v1"
	OwnershipFullFile = "full_file"

	ArchiveSkillResourceID = "opencode.skill/sdd-archive@1"
	ArchiveSkillAssetPath  = "skills/sdd-archive/SKILL.md"
	ArchiveSkillTargetPath = ".config/opencode/skills/sdd-archive/SKILL.md"
)

type Producer struct {
	Version       string `json:"version"`
	VCSRevision   string `json:"vcs_revision,omitempty"`
	CatalogDigest string `json:"catalog_digest"`
}

type Resource struct {
	ResourceID     string `json:"resource_id"`
	CanonicalPath  string `json:"canonical_path"`
	Ownership      string `json:"ownership"`
	DesiredSHA256  string `json:"desired_sha256"`
	ObservedSHA256 string `json:"observed_sha256"`
	Mode           uint32 `json:"mode"`
}

type Manifest struct {
	Schema        string     `json:"schema"`
	Generation    uint64     `json:"generation"`
	TransactionID string     `json:"transaction_id"`
	Producer      Producer   `json:"producer"`
	Resources     []Resource `json:"resources"`
}

type Descriptor struct {
	ResourceID    string
	CanonicalPath string
	Ownership     string
	Content       []byte
	Mode          uint32
}

func Catalog() ([]Descriptor, error) {
	content, err := assets.Read(ArchiveSkillAssetPath)
	if err != nil {
		return nil, fmt.Errorf("read managed resource %q: %w", ArchiveSkillResourceID, err)
	}
	return []Descriptor{{
		ResourceID:    ArchiveSkillResourceID,
		CanonicalPath: ArchiveSkillTargetPath,
		Ownership:     OwnershipFullFile,
		Content:       []byte(content),
		Mode:          0o644,
	}}, nil
}

func ResolveTarget(homeDir string, descriptor Descriptor) string {
	return filepath.Join(homeDir, filepath.FromSlash(descriptor.CanonicalPath))
}

func Digest(descriptors []Descriptor) (string, error) {
	canonical := append([]Descriptor(nil), descriptors...)
	sort.Slice(canonical, func(i, j int) bool { return canonical[i].ResourceID < canonical[j].ResourceID })
	hash := sha256.New()
	encoder := json.NewEncoder(hash)
	encoder.SetEscapeHTML(false)
	for _, descriptor := range canonical {
		entry := struct {
			ResourceID    string `json:"resource_id"`
			CanonicalPath string `json:"canonical_path"`
			Ownership     string `json:"ownership"`
			DesiredSHA256 string `json:"desired_sha256"`
			Mode          uint32 `json:"mode"`
		}{
			ResourceID:    descriptor.ResourceID,
			CanonicalPath: descriptor.CanonicalPath,
			Ownership:     descriptor.Ownership,
			DesiredSHA256: SHA256(descriptor.Content),
			Mode:          descriptor.Mode,
		}
		if err := encoder.Encode(entry); err != nil {
			return "", fmt.Errorf("encode catalog entry %q: %w", descriptor.ResourceID, err)
		}
	}
	return "sha256:" + hex.EncodeToString(hash.Sum(nil)), nil
}

func SHA256(content []byte) string {
	digest := sha256.Sum256(content)
	return "sha256:" + hex.EncodeToString(digest[:])
}

func MarshalManifest(manifest Manifest) ([]byte, error) {
	resources := append([]Resource(nil), manifest.Resources...)
	sort.Slice(resources, func(i, j int) bool { return resources[i].ResourceID < resources[j].ResourceID })
	manifest.Resources = resources
	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(data, '\n'), nil
}
