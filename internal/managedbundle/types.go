package managedbundle

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"path/filepath"
	"sort"

	"github.com/gentleman-programming/gentle-ai/v2/internal/model"
)

const (
	ManifestSchemaV1 = "gentle-ai.managed-bundle/v1"
	JournalSchemaV1  = "gentle-ai.managed-journal/v1"

	ManagedDirName   = ".gentle-ai"
	ManagedSubDir    = "managed/v1"
	ManifestFileName = "manifest.json"
	JournalDirName   = "transactions"
	JournalFileName  = "journal.json"
)

// OwnershipKind identifies the extent of managed ownership over a target file.
type OwnershipKind string

const (
	OwnershipFullFile OwnershipKind = "full_file"
)

// BundleIdentity is the top-level bundle coherence classification for doctor.
type BundleIdentity string

const (
	BundleIdentityAligned      BundleIdentity = "aligned"
	BundleIdentityStale        BundleIdentity = "stale"
	BundleIdentityMixed        BundleIdentity = "mixed"
	BundleIdentityUserModified BundleIdentity = "user_modified"
	BundleIdentityUnknown      BundleIdentity = "unknown"
)

// ExtentIntegrity represents the physical state of the managed files on disk.
type ExtentIntegrity string

const (
	ExtentIntegrityMatch        ExtentIntegrity = "match"
	ExtentIntegrityUserModified ExtentIntegrity = "user_modified"
	ExtentIntegrityMissing      ExtentIntegrity = "missing"
	ExtentIntegrityTypeChanged  ExtentIntegrity = "type_changed"
)

// RecoveryState represents the transaction recovery state overlay.
type RecoveryState string

const (
	RecoveryStateNone             RecoveryState = "none"
	RecoveryStateResumableBefore  RecoveryState = "resumable_before"
	RecoveryStateResumableDesired RecoveryState = "resumable_desired"
	RecoveryStateBlockedConflict  RecoveryState = "blocked_conflict"
	RecoveryStateRollbackRequired RecoveryState = "rollback_required"
	RecoveryStateRolledBack       RecoveryState = "rolled_back"
)

// JournalPhase represents the recorded phase of an active transaction.
type JournalPhase string

const (
	PhasePlanned           JournalPhase = "planned"
	PhaseDiscovered        JournalPhase = "discovered"
	PhaseClassified        JournalPhase = "classified"
	PhaseSnapshotted       JournalPhase = "snapshotted"
	PhasePrepared          JournalPhase = "prepared"
	PhaseApplying          JournalPhase = "applying"
	PhaseVerified          JournalPhase = "verified"
	PhaseManifestCommitted JournalPhase = "manifest_committed"
	PhaseCompleted         JournalPhase = "completed"
	PhaseRolledBack        JournalPhase = "rolled_back"
)

// ProducerInfo identifies the binary that created or owns the manifest.
type ProducerInfo struct {
	Version       string `json:"version"`
	VCSRevision   string `json:"vcs_revision,omitempty"`
	CatalogDigest string `json:"catalog_digest"`
}

// OwnershipDescriptor describes how a resource is owned.
type OwnershipDescriptor struct {
	Kind OwnershipKind `json:"kind"`
}

// ResourceEntry records one managed resource in the committed manifest.
type ResourceEntry struct {
	ResourceID     string              `json:"resource_id"`
	TargetIdentity string              `json:"target_identity"`
	CanonicalPath  string              `json:"canonical_path,omitempty"`
	Ownership      OwnershipDescriptor `json:"ownership"`
	DesiredSHA256  string              `json:"desired_sha256"`
	ObservedSHA256 string              `json:"observed_sha256"`
	Mode           uint32              `json:"mode"`
	BackupRef      string              `json:"backup_ref,omitempty"`
	TransactionID  string              `json:"transaction_id,omitempty"`
}

// ManagedBundleManifest records the durable committed state of all managed assets.
type ManagedBundleManifest struct {
	Schema        string          `json:"schema"`
	Generation    uint64          `json:"generation"`
	TransactionID string          `json:"transaction_id"`
	Producer      ProducerInfo    `json:"producer"`
	Resources     []ResourceEntry `json:"resources"`
}

// ResourceJournalEntry records the transition intent and observation for one resource.
type ResourceJournalEntry struct {
	ResourceID     string              `json:"resource_id"`
	CanonicalPath  string              `json:"canonical_path"`
	Ownership      OwnershipDescriptor `json:"ownership"`
	BeforeSHA256   string              `json:"before_sha256"`
	BeforeMode     uint32              `json:"before_mode"`
	DesiredSHA256  string              `json:"desired_sha256"`
	DesiredMode    uint32              `json:"desired_mode"`
	ObservedSHA256 string              `json:"observed_sha256,omitempty"`
	ObservedMode   uint32              `json:"observed_mode,omitempty"`
	Applied        bool                `json:"applied,omitempty"`
}

// MigrationJournal records a transaction write-ahead log for crash recovery.
type MigrationJournal struct {
	Schema             string                 `json:"schema"`
	TransactionID      string                 `json:"transaction_id"`
	ExpectedGeneration uint64                 `json:"expected_generation"`
	ProposedGeneration uint64                 `json:"proposed_generation"`
	ExpectedDigest     string                 `json:"expected_digest,omitempty"`
	ProposedDigest     string                 `json:"proposed_digest,omitempty"`
	BackupRef          string                 `json:"backup_ref,omitempty"`
	LastPhase          JournalPhase           `json:"last_phase"`
	Resources          []ResourceJournalEntry `json:"resources"`
}

// SyncReason represents the typed reason code for a sync eligibility determination.
type SyncReason string

const (
	SyncReasonNoManifest                  SyncReason = "no_manifest"
	SyncReasonManifestUnreadable          SyncReason = "manifest_unreadable"
	SyncReasonManifestMalformed           SyncReason = "manifest_malformed"
	SyncReasonUnsupportedSchema           SyncReason = "unsupported_schema"
	SyncReasonUnresolvedTransaction       SyncReason = "unresolved_transaction"
	SyncReasonEmptyManifest               SyncReason = "empty_manifest"
	SyncReasonMixedResourceTransactions   SyncReason = "mixed_resource_transactions"
	SyncReasonResourceMissing             SyncReason = "resource_missing"
	SyncReasonResourceUnreadable          SyncReason = "resource_unreadable"
	SyncReasonTypeChanged                 SyncReason = "type_changed"
	SyncReasonContentMismatch             SyncReason = "content_mismatch"
	SyncReasonModeMismatch                SyncReason = "mode_mismatch"
	SyncReasonCatalogDigestError          SyncReason = "catalog_digest_error"
	SyncReasonAlignedWithCurrentBinary    SyncReason = "aligned_with_current_binary"
	SyncReasonExactCommittedOlderBundle   SyncReason = "exact_committed_older_bundle"
)

// ClassificationResult represents the evaluated doctor diagnostic outcome.
type ClassificationResult struct {
	BundleIdentity     BundleIdentity  `json:"bundle_identity"`
	ExtentIntegrity    ExtentIntegrity `json:"extent_integrity"`
	RecoveryState      RecoveryState   `json:"recovery_state"`
	SyncEligible       bool            `json:"sync_eligible"`
	SyncEligibleReason SyncReason      `json:"sync_eligible_reason"`
	Detail             string          `json:"detail"`
	UnsupportedSchema  bool            `json:"unsupported_schema,omitempty"`
}

// DesiredExtent represents the rendered content and desired metadata of a managed resource.
type DesiredExtent struct {
	Content []byte
	SHA256  string
	Mode    uint32
}

// ResourceDescriptor describes a compile-time managed resource definition.
type ResourceDescriptor struct {
	ResourceID    string
	SchemaVersion uint32
	ComponentID   model.ComponentID
	AgentID       model.AgentID
	CanonicalPath string // Relative to user home dir, forward-slash normalized
	Ownership     OwnershipDescriptor
	RenderDesired func() (DesiredExtent, error)
}

// ResourceCatalog is the compile-time collection of desired managed resources.
type ResourceCatalog struct {
	Version     string
	VCSRevision string
	Resources   []ResourceDescriptor
}

// ComputeCatalogDigest calculates a deterministic sha256 hash over descriptor metadata and desired extents.
// Note: c.Version and c.VCSRevision are intentionally excluded from the digest calculation so that
// the digest reflects resource metadata and rendered content only, allowing two binaries built
// with identical assets to produce the exact same catalog digest regardless of VCS stamp or build metadata.
func (c ResourceCatalog) ComputeCatalogDigest() (string, error) {
	sortedResources := make([]ResourceDescriptor, len(c.Resources))
	copy(sortedResources, c.Resources)
	sort.Slice(sortedResources, func(i, j int) bool {
		return sortedResources[i].ResourceID < sortedResources[j].ResourceID
	})

	h := sha256.New()
	for _, r := range sortedResources {
		desired, err := r.RenderDesired()
		if err != nil {
			return "", fmt.Errorf("render desired extent for %s: %w", r.ResourceID, err)
		}
		digest := desired.SHA256
		if digest == "" {
			sum := sha256.Sum256(desired.Content)
			digest = "sha256:" + hex.EncodeToString(sum[:])
		}
		// Write canonical metadata
		entryLine := fmt.Sprintf("%s|%d|%s|%s|%s|%s|%s|%d\n",
			r.ResourceID,
			r.SchemaVersion,
			r.ComponentID,
			r.AgentID,
			filepath.ToSlash(r.CanonicalPath),
			r.Ownership.Kind,
			digest,
			desired.Mode,
		)
		h.Write([]byte(entryLine))
	}
	return "sha256:" + hex.EncodeToString(h.Sum(nil)), nil
}

// TargetIdentityToken computes the canonical target identity token for a relative path.
func TargetIdentityToken(relPath string) string {
	normalized := filepath.ToSlash(relPath)
	sum := sha256.Sum256([]byte(normalized))
	return "sha256:" + hex.EncodeToString(sum[:])
}
