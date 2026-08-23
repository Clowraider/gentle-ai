package managedbundle

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

const ManifestFile = ".gentle-ai/managed-bundle.json"

const (
	JournalSchemaV1 = "gentle-ai.managed-bundle-journal/v1"
	JournalDir      = ".gentle-ai/transactions"
)

type BundleIdentity string

const (
	BundleAligned      BundleIdentity = "aligned"
	BundleStale        BundleIdentity = "stale"
	BundleUserModified BundleIdentity = "user_modified"
	BundleUnknown      BundleIdentity = "unknown"
)

type ExtentIntegrity string

const (
	ExtentMatch        ExtentIntegrity = "match"
	ExtentUserModified ExtentIntegrity = "user_modified"
	ExtentMissing      ExtentIntegrity = "missing"
	ExtentTypeChanged  ExtentIntegrity = "type_changed"
	ExtentUnknown      ExtentIntegrity = "unknown"
)

type Classification struct {
	BundleIdentity  BundleIdentity
	ExtentIntegrity ExtentIntegrity
	RecoveryState   RecoveryState
	SyncEligible    bool
	Detail          string
}

type RecoveryState string

const (
	RecoveryNone             RecoveryState = "none"
	RecoveryResumableBefore  RecoveryState = "resumable_before"
	RecoveryResumableDesired RecoveryState = "resumable_desired"
	RecoveryBlocked          RecoveryState = "blocked_conflict"
)

type JournalResource struct {
	ResourceID    string `json:"resource_id"`
	CanonicalPath string `json:"canonical_path"`
	BeforeSHA256  string `json:"before_sha256"`
	BeforeMode    uint32 `json:"before_mode"`
	DesiredSHA256 string `json:"desired_sha256"`
	DesiredMode   uint32 `json:"desired_mode"`
}

type Journal struct {
	Schema             string            `json:"schema"`
	TransactionID      string            `json:"transaction_id"`
	ExpectedGeneration uint64            `json:"expected_generation"`
	ProposedGeneration uint64            `json:"proposed_generation"`
	Phase              string            `json:"phase"`
	Resources          []JournalResource `json:"resources"`
}

func ManifestPath(homeDir string) string {
	return filepath.Join(homeDir, filepath.FromSlash(ManifestFile))
}

func ClassifyExtents(homeDir, version, revision string, descriptors []Descriptor) Classification {
	manifest, err := readManifest(homeDir)
	if err != nil {
		return Classification{BundleIdentity: BundleUnknown, ExtentIntegrity: ExtentUnknown, Detail: err.Error()}
	}
	if manifest.Schema != ManifestSchemaV1 || len(manifest.Resources) != len(descriptors) || len(descriptors) == 0 {
		return Classification{BundleIdentity: BundleUnknown, ExtentIntegrity: ExtentUnknown, Detail: "managed bundle manifest is unsupported or incomplete"}
	}
	if recovery, detail := classifyRecovery(homeDir, manifest); recovery != RecoveryNone {
		return Classification{BundleIdentity: BundleUnknown, ExtentIntegrity: ExtentUnknown, RecoveryState: recovery, Detail: detail}
	}

	digest, err := Digest(descriptors)
	if err != nil {
		return Classification{BundleIdentity: BundleUnknown, ExtentIntegrity: ExtentUnknown, Detail: err.Error()}
	}
	committed := make(map[string]Resource, len(manifest.Resources))
	for _, resource := range manifest.Resources {
		if resource.ResourceID == "" || resource.DesiredSHA256 != resource.ObservedSHA256 {
			return Classification{BundleIdentity: BundleUnknown, ExtentIntegrity: ExtentUnknown, Detail: "managed bundle manifest is internally inconsistent"}
		}
		committed[resource.ResourceID] = resource
	}

	for _, descriptor := range descriptors {
		resource, ok := committed[descriptor.ResourceID]
		if !ok || resource.CanonicalPath != descriptor.CanonicalPath || resource.Ownership != descriptor.Ownership || resource.Mode != descriptor.Mode {
			return Classification{BundleIdentity: BundleUnknown, ExtentIntegrity: ExtentUnknown, Detail: "managed resource descriptor does not match the running catalog"}
		}
		path, err := safeTarget(homeDir, resource.CanonicalPath)
		if err != nil {
			return Classification{BundleIdentity: BundleUserModified, ExtentIntegrity: ExtentTypeChanged, Detail: err.Error()}
		}
		info, err := os.Lstat(path)
		if errors.Is(err, os.ErrNotExist) {
			return Classification{BundleIdentity: BundleUserModified, ExtentIntegrity: ExtentMissing, Detail: fmt.Sprintf("managed resource %s is missing", resource.ResourceID)}
		}
		if err != nil {
			return Classification{BundleIdentity: BundleUnknown, ExtentIntegrity: ExtentUnknown, Detail: fmt.Sprintf("inspect managed resource %s: %v", resource.ResourceID, err)}
		}
		if !info.Mode().IsRegular() {
			return Classification{BundleIdentity: BundleUserModified, ExtentIntegrity: ExtentTypeChanged, Detail: fmt.Sprintf("managed resource %s is not a regular file", resource.ResourceID)}
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return Classification{BundleIdentity: BundleUnknown, ExtentIntegrity: ExtentUnknown, Detail: fmt.Sprintf("read managed resource %s: %v", resource.ResourceID, err)}
		}
		if SHA256(content) != resource.ObservedSHA256 || uint32(info.Mode().Perm()) != resource.Mode {
			return Classification{BundleIdentity: BundleUserModified, ExtentIntegrity: ExtentUserModified, Detail: fmt.Sprintf("managed resource %s differs from its committed extent", resource.ResourceID)}
		}
	}

	current := manifest.Producer.Version == version && manifest.Producer.CatalogDigest == digest && (revision == "" || manifest.Producer.VCSRevision == revision)
	if current {
		for _, descriptor := range descriptors {
			if committed[descriptor.ResourceID].DesiredSHA256 != SHA256(descriptor.Content) {
				return Classification{BundleIdentity: BundleUnknown, ExtentIntegrity: ExtentMatch, Detail: "committed resource identity does not match the running catalog"}
			}
		}
		return Classification{BundleIdentity: BundleAligned, ExtentIntegrity: ExtentMatch, Detail: "managed bundle matches the running binary"}
	}
	if manifest.Producer.Version == "" || manifest.Producer.CatalogDigest == "" {
		return Classification{BundleIdentity: BundleUnknown, ExtentIntegrity: ExtentMatch, Detail: "managed bundle producer identity is incomplete"}
	}
	return Classification{BundleIdentity: BundleStale, ExtentIntegrity: ExtentMatch, SyncEligible: true, Detail: "managed bundle is a complete older installation; run `gentle-ai sync`"}
}

func classifyRecovery(homeDir string, manifest Manifest) (RecoveryState, string) {
	root := filepath.Join(homeDir, filepath.FromSlash(JournalDir))
	entries, err := os.ReadDir(root)
	if errors.Is(err, os.ErrNotExist) {
		return RecoveryNone, ""
	}
	if err != nil {
		return RecoveryBlocked, fmt.Sprintf("read managed transaction directory: %v", err)
	}
	selected := RecoveryNone
	for _, entry := range entries {
		if !entry.IsDir() || !validTransactionID(entry.Name()) {
			return RecoveryBlocked, "managed transaction entry is unsafe"
		}
		journal, err := readJournal(filepath.Join(root, entry.Name(), "journal.json"))
		if err != nil || journal.Schema != JournalSchemaV1 || journal.TransactionID != entry.Name() || len(journal.Resources) == 0 {
			return RecoveryBlocked, "managed transaction journal is malformed or unsupported"
		}
		if journal.Phase == "completed" || journal.Phase == "rolled_back" || manifest.TransactionID == journal.TransactionID {
			continue
		}
		if journal.ExpectedGeneration != manifest.Generation || journal.ProposedGeneration != manifest.Generation+1 {
			return RecoveryBlocked, "managed transaction generation does not match the committed manifest"
		}
		state := classifyJournalResources(homeDir, journal.Resources)
		if state == RecoveryBlocked || selected != RecoveryNone && selected != state {
			return RecoveryBlocked, "managed transaction extents conflict"
		}
		selected = state
	}
	if selected == RecoveryNone {
		return RecoveryNone, ""
	}
	return selected, "managed transaction requires read-only recovery classification"
}

func classifyJournalResources(homeDir string, resources []JournalResource) RecoveryState {
	allBefore, allDesired := true, true
	for _, resource := range resources {
		target, err := safeTarget(homeDir, resource.CanonicalPath)
		if err != nil {
			return RecoveryBlocked
		}
		info, err := os.Lstat(target)
		if err != nil || !info.Mode().IsRegular() {
			return RecoveryBlocked
		}
		content, err := os.ReadFile(target)
		if err != nil {
			return RecoveryBlocked
		}
		digest, mode := SHA256(content), uint32(info.Mode().Perm())
		before := digest == resource.BeforeSHA256 && mode == resource.BeforeMode
		desired := digest == resource.DesiredSHA256 && mode == resource.DesiredMode
		allBefore, allDesired = allBefore && before, allDesired && desired
		if !before && !desired {
			return RecoveryBlocked
		}
	}
	if allDesired {
		return RecoveryResumableDesired
	}
	if allBefore {
		return RecoveryResumableBefore
	}
	return RecoveryBlocked
}

func readJournal(path string) (Journal, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Journal{}, err
	}
	decoder := json.NewDecoder(strings.NewReader(string(data)))
	decoder.DisallowUnknownFields()
	var journal Journal
	if err := decoder.Decode(&journal); err != nil {
		return Journal{}, err
	}
	var extra any
	if decoder.Decode(&extra) != io.EOF {
		return Journal{}, errors.New("trailing journal data")
	}
	return journal, nil
}

func validTransactionID(id string) bool {
	return id != "" && id != "." && id != ".." && filepath.Base(id) == id && !strings.ContainsAny(id, `/\\`)
}

func readManifest(homeDir string) (Manifest, error) {
	data, err := os.ReadFile(ManifestPath(homeDir))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return Manifest{}, errors.New("managed bundle identity is unknown: no manifest")
		}
		return Manifest{}, fmt.Errorf("read managed bundle manifest: %w", err)
	}
	decoder := json.NewDecoder(strings.NewReader(string(data)))
	decoder.DisallowUnknownFields()
	var manifest Manifest
	if err := decoder.Decode(&manifest); err != nil {
		return Manifest{}, fmt.Errorf("decode managed bundle manifest: %w", err)
	}
	var extra any
	if decoder.Decode(&extra) != io.EOF {
		return Manifest{}, errors.New("decode managed bundle manifest: trailing data")
	}
	return manifest, nil
}

func safeTarget(homeDir, canonicalPath string) (string, error) {
	if strings.TrimSpace(canonicalPath) == "" || filepath.IsAbs(canonicalPath) {
		return "", errors.New("managed resource path is unsafe")
	}
	cleanHome := filepath.Clean(homeDir)
	target := filepath.Join(cleanHome, filepath.FromSlash(canonicalPath))
	relative, err := filepath.Rel(cleanHome, target)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", errors.New("managed resource path escapes home")
	}
	return target, nil
}
