package managedbundle

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/model"
)

func makeTestCatalog(version string, content []byte, mode uint32) ResourceCatalog {
	sum := sha256.Sum256(content)
	contentSHA := "sha256:" + hex.EncodeToString(sum[:])

	return ResourceCatalog{
		Version:     version,
		VCSRevision: "commit-" + version,
		Resources: []ResourceDescriptor{
			{
				ResourceID:    DefaultArchiveSkillResourceID,
				SchemaVersion: 1,
				ComponentID:   model.ComponentSkills,
				AgentID:       model.AgentOpenCode,
				CanonicalPath: DefaultArchiveSkillRelPath,
				Ownership:     OwnershipDescriptor{Kind: OwnershipFullFile},
				RenderDesired: func() (DesiredExtent, error) {
					return DesiredExtent{
						Content: content,
						SHA256:  contentSHA,
						Mode:    mode,
					}, nil
				},
			},
		},
	}
}

// TestFixture1_B2Manifest_ExactB2Bytes_Aligned verifies:
// 1. B2 manifest + exact B2 bytes/mode -> aligned, eligible no-op.
func TestFixture1_B2Manifest_ExactB2Bytes_Aligned(t *testing.T) {
	home := t.TempDir()
	b2Bytes := []byte("# sdd-archive skill v2\n")
	b2Mode := uint32(0644)
	catalogB2 := makeTestCatalog("2.2.0", b2Bytes, b2Mode)
	b2Digest, err := catalogB2.ComputeCatalogDigest()
	if err != nil {
		t.Fatalf("compute digest: %v", err)
	}

	// Seed file on disk
	filePath := filepath.Join(home, filepath.FromSlash(DefaultArchiveSkillRelPath))
	if err := os.MkdirAll(filepath.Dir(filePath), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filePath, b2Bytes, 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(filePath, 0644); err != nil {
		t.Fatal(err)
	}

	sum := sha256.Sum256(b2Bytes)
	sha := "sha256:" + hex.EncodeToString(sum[:])

	manifest := ManagedBundleManifest{
		Schema:        ManifestSchemaV1,
		Generation:    2,
		TransactionID: "tx-2",
		Producer: ProducerInfo{
			Version:       "2.2.0",
			VCSRevision:   "commit-2.2.0",
			CatalogDigest: b2Digest,
		},
		Resources: []ResourceEntry{
			{
				ResourceID:     DefaultArchiveSkillResourceID,
				TargetIdentity: TargetIdentityToken(DefaultArchiveSkillRelPath),
				CanonicalPath:  DefaultArchiveSkillRelPath,
				Ownership:      OwnershipDescriptor{Kind: OwnershipFullFile},
				DesiredSHA256:  sha,
				ObservedSHA256: sha,
				Mode:           b2Mode,
				TransactionID:  "tx-2",
			},
		},
	}
	if err := WriteManifest(home, manifest); err != nil {
		t.Fatal(err)
	}

	res := Classify(context.Background(), home, catalogB2)
	if res.BundleIdentity != BundleIdentityAligned {
		t.Errorf("got bundle identity %s, want %s", res.BundleIdentity, BundleIdentityAligned)
	}
	if res.ExtentIntegrity != ExtentIntegrityMatch {
		t.Errorf("got extent integrity %s, want %s", res.ExtentIntegrity, ExtentIntegrityMatch)
	}
	if res.RecoveryState != RecoveryStateNone {
		t.Errorf("got recovery state %s, want %s", res.RecoveryState, RecoveryStateNone)
	}
	if !res.SyncEligible {
		t.Errorf("expected sync eligible")
	}
}

// TestFixture2_B1Manifest_ExactB1Bytes_Stale verifies:
// 2. B1 manifest + exact B1 bytes/mode under B2 -> stale, sync eligible.
func TestFixture2_B1Manifest_ExactB1Bytes_Stale(t *testing.T) {
	home := t.TempDir()
	b1Bytes := []byte("# sdd-archive skill v1\n")
	b1Mode := uint32(0644)
	catalogB1 := makeTestCatalog("2.1.10", b1Bytes, b1Mode)
	b1Digest, err := catalogB1.ComputeCatalogDigest()
	if err != nil {
		t.Fatalf("compute b1 digest: %v", err)
	}

	b2Bytes := []byte("# sdd-archive skill v2\n")
	b2Mode := uint32(0644)
	catalogB2 := makeTestCatalog("2.2.0", b2Bytes, b2Mode)

	// File on disk is B1
	filePath := filepath.Join(home, filepath.FromSlash(DefaultArchiveSkillRelPath))
	if err := os.MkdirAll(filepath.Dir(filePath), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filePath, b1Bytes, 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(filePath, 0644); err != nil {
		t.Fatal(err)
	}

	sum := sha256.Sum256(b1Bytes)
	sha := "sha256:" + hex.EncodeToString(sum[:])

	manifest := ManagedBundleManifest{
		Schema:        ManifestSchemaV1,
		Generation:    1,
		TransactionID: "tx-1",
		Producer: ProducerInfo{
			Version:       "2.1.10",
			VCSRevision:   "commit-2.1.10",
			CatalogDigest: b1Digest,
		},
		Resources: []ResourceEntry{
			{
				ResourceID:     DefaultArchiveSkillResourceID,
				TargetIdentity: TargetIdentityToken(DefaultArchiveSkillRelPath),
				CanonicalPath:  DefaultArchiveSkillRelPath,
				Ownership:      OwnershipDescriptor{Kind: OwnershipFullFile},
				DesiredSHA256:  sha,
				ObservedSHA256: sha,
				Mode:           b1Mode,
				TransactionID:  "tx-1",
			},
		},
	}
	if err := WriteManifest(home, manifest); err != nil {
		t.Fatal(err)
	}

	res := Classify(context.Background(), home, catalogB2)
	if res.BundleIdentity != BundleIdentityStale {
		t.Errorf("got bundle identity %s, want %s", res.BundleIdentity, BundleIdentityStale)
	}
	if res.ExtentIntegrity != ExtentIntegrityMatch {
		t.Errorf("got extent integrity %s, want %s", res.ExtentIntegrity, ExtentIntegrityMatch)
	}
	if !res.SyncEligible || res.SyncEligibleReason != SyncReasonExactCommittedOlderBundle {
		t.Errorf("got sync eligible %v (%s), want true (%s)", res.SyncEligible, res.SyncEligibleReason, SyncReasonExactCommittedOlderBundle)
	}
}

// TestFixture3_B1Manifest_ChangedByte_UserModified verifies:
// 3. B1 manifest + one changed byte or mode -> user_modified, not eligible.
func TestFixture3_B1Manifest_ChangedByte_UserModified(t *testing.T) {
	home := t.TempDir()
	b1Bytes := []byte("# sdd-archive skill v1\n")
	b1Mode := uint32(0644)
	catalogB1 := makeTestCatalog("2.1.10", b1Bytes, b1Mode)
	b1Digest, err := catalogB1.ComputeCatalogDigest()
	if err != nil {
		t.Fatalf("compute b1 digest: %v", err)
	}

	catalogB2 := makeTestCatalog("2.2.0", []byte("# sdd-archive skill v2\n"), 0644)

	// User modified the file on disk
	filePath := filepath.Join(home, filepath.FromSlash(DefaultArchiveSkillRelPath))
	if err := os.MkdirAll(filepath.Dir(filePath), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filePath, []byte("# user edited content\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(filePath, 0644); err != nil {
		t.Fatal(err)
	}

	sum := sha256.Sum256(b1Bytes)
	sha := "sha256:" + hex.EncodeToString(sum[:])

	manifest := ManagedBundleManifest{
		Schema:        ManifestSchemaV1,
		Generation:    1,
		TransactionID: "tx-1",
		Producer: ProducerInfo{
			Version:       "2.1.10",
			VCSRevision:   "commit-2.1.10",
			CatalogDigest: b1Digest,
		},
		Resources: []ResourceEntry{
			{
				ResourceID:     DefaultArchiveSkillResourceID,
				TargetIdentity: TargetIdentityToken(DefaultArchiveSkillRelPath),
				CanonicalPath:  DefaultArchiveSkillRelPath,
				Ownership:      OwnershipDescriptor{Kind: OwnershipFullFile},
				DesiredSHA256:  sha,
				ObservedSHA256: sha,
				Mode:           b1Mode,
				TransactionID:  "tx-1",
			},
		},
	}
	if err := WriteManifest(home, manifest); err != nil {
		t.Fatal(err)
	}

	res := Classify(context.Background(), home, catalogB2)
	if res.BundleIdentity != BundleIdentityUserModified {
		t.Errorf("got bundle identity %s, want %s", res.BundleIdentity, BundleIdentityUserModified)
	}
	if res.ExtentIntegrity != ExtentIntegrityUserModified {
		t.Errorf("got extent integrity %s, want %s", res.ExtentIntegrity, ExtentIntegrityUserModified)
	}
	if res.SyncEligible {
		t.Errorf("expected not sync eligible")
	}
}

// TestFixture4_NoManifest_Unknown verifies:
// 4. No manifest + bytes equal B1 or B2 -> unknown, not eligible.
func TestFixture4_NoManifest_Unknown(t *testing.T) {
	home := t.TempDir()
	b2Bytes := []byte("# sdd-archive skill v2\n")
	catalogB2 := makeTestCatalog("2.2.0", b2Bytes, 0644)

	// File exists on disk matching B2, but no manifest exists
	filePath := filepath.Join(home, filepath.FromSlash(DefaultArchiveSkillRelPath))
	if err := os.MkdirAll(filepath.Dir(filePath), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filePath, b2Bytes, 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(filePath, 0644); err != nil {
		t.Fatal(err)
	}

	res := Classify(context.Background(), home, catalogB2)
	if res.BundleIdentity != BundleIdentityUnknown {
		t.Errorf("got bundle identity %s, want %s", res.BundleIdentity, BundleIdentityUnknown)
	}
	if res.SyncEligible {
		t.Errorf("expected not sync eligible")
	}
}

// TestFixture5_PreparedTransaction_ObservedBefore_Recoverable verifies:
// 5. Prepared transaction + observed before -> recoverable (resumable_before), not yet aligned.
func TestFixture5_PreparedTransaction_ObservedBefore_Recoverable(t *testing.T) {
	home := t.TempDir()
	b1Bytes := []byte("# sdd-archive skill v1\n")
	b2Bytes := []byte("# sdd-archive skill v2\n")
	catalogB2 := makeTestCatalog("2.2.0", b2Bytes, 0644)

	filePath := filepath.Join(home, filepath.FromSlash(DefaultArchiveSkillRelPath))
	if err := os.MkdirAll(filepath.Dir(filePath), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filePath, b1Bytes, 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(filePath, 0644); err != nil {
		t.Fatal(err)
	}

	sum1 := sha256.Sum256(b1Bytes)
	sha1 := "sha256:" + hex.EncodeToString(sum1[:])
	sum2 := sha256.Sum256(b2Bytes)
	sha2 := "sha256:" + hex.EncodeToString(sum2[:])

	manifest := ManagedBundleManifest{
		Schema:        ManifestSchemaV1,
		Generation:    1,
		TransactionID: "tx-1",
		Producer: ProducerInfo{
			Version:       "2.1.10",
			CatalogDigest: "sha256:old",
		},
		Resources: []ResourceEntry{
			{
				ResourceID:     DefaultArchiveSkillResourceID,
				TargetIdentity: TargetIdentityToken(DefaultArchiveSkillRelPath),
				CanonicalPath:  DefaultArchiveSkillRelPath,
				Ownership:      OwnershipDescriptor{Kind: OwnershipFullFile},
				DesiredSHA256:  sha1,
				ObservedSHA256: sha1,
				Mode:           0644,
				TransactionID:  "tx-1",
			},
		},
	}
	if err := WriteManifest(home, manifest); err != nil {
		t.Fatal(err)
	}

	journal := MigrationJournal{
		Schema:             JournalSchemaV1,
		TransactionID:      "tx-2",
		ExpectedGeneration: 1,
		ProposedGeneration: 2,
		LastPhase:          PhasePrepared,
		Resources: []ResourceJournalEntry{
			{
				ResourceID:    DefaultArchiveSkillResourceID,
				CanonicalPath: DefaultArchiveSkillRelPath,
				Ownership:     OwnershipDescriptor{Kind: OwnershipFullFile},
				BeforeSHA256:  sha1,
				BeforeMode:    0644,
				DesiredSHA256: sha2,
				DesiredMode:   0644,
			},
		},
	}
	if err := WriteJournal(home, journal); err != nil {
		t.Fatal(err)
	}

	res := Classify(context.Background(), home, catalogB2)
	if res.RecoveryState != RecoveryStateResumableBefore {
		t.Errorf("got recovery state %s, want %s", res.RecoveryState, RecoveryStateResumableBefore)
	}
	if res.SyncEligible {
		t.Errorf("expected not sync eligible")
	}
}

// TestFixture6_PreparedTransaction_ObservedDesired_ResumableCommit verifies:
// 6. Prepared transaction + observed desired -> resumable commit (resumable_desired), not a second mutation.
func TestFixture6_PreparedTransaction_ObservedDesired_ResumableCommit(t *testing.T) {
	home := t.TempDir()
	b1Bytes := []byte("# sdd-archive skill v1\n")
	b2Bytes := []byte("# sdd-archive skill v2\n")
	catalogB2 := makeTestCatalog("2.2.0", b2Bytes, 0644)

	filePath := filepath.Join(home, filepath.FromSlash(DefaultArchiveSkillRelPath))
	if err := os.MkdirAll(filepath.Dir(filePath), 0755); err != nil {
		t.Fatal(err)
	}
	// Desired bytes already written to disk before crash
	if err := os.WriteFile(filePath, b2Bytes, 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(filePath, 0644); err != nil {
		t.Fatal(err)
	}

	sum1 := sha256.Sum256(b1Bytes)
	sha1 := "sha256:" + hex.EncodeToString(sum1[:])
	sum2 := sha256.Sum256(b2Bytes)
	sha2 := "sha256:" + hex.EncodeToString(sum2[:])

	manifest := ManagedBundleManifest{
		Schema:        ManifestSchemaV1,
		Generation:    1,
		TransactionID: "tx-1",
		Producer: ProducerInfo{
			Version:       "2.1.10",
			CatalogDigest: "sha256:old",
		},
		Resources: []ResourceEntry{
			{
				ResourceID:     DefaultArchiveSkillResourceID,
				TargetIdentity: TargetIdentityToken(DefaultArchiveSkillRelPath),
				CanonicalPath:  DefaultArchiveSkillRelPath,
				Ownership:      OwnershipDescriptor{Kind: OwnershipFullFile},
				DesiredSHA256:  sha1,
				ObservedSHA256: sha1,
				Mode:           0644,
				TransactionID:  "tx-1",
			},
		},
	}
	if err := WriteManifest(home, manifest); err != nil {
		t.Fatal(err)
	}

	journal := MigrationJournal{
		Schema:             JournalSchemaV1,
		TransactionID:      "tx-2",
		ExpectedGeneration: 1,
		ProposedGeneration: 2,
		LastPhase:          PhaseApplying,
		Resources: []ResourceJournalEntry{
			{
				ResourceID:    DefaultArchiveSkillResourceID,
				CanonicalPath: DefaultArchiveSkillRelPath,
				Ownership:     OwnershipDescriptor{Kind: OwnershipFullFile},
				BeforeSHA256:  sha1,
				BeforeMode:    0644,
				DesiredSHA256: sha2,
				DesiredMode:   0644,
				Applied:       true,
			},
		},
	}
	if err := WriteJournal(home, journal); err != nil {
		t.Fatal(err)
	}

	res := Classify(context.Background(), home, catalogB2)
	if res.RecoveryState != RecoveryStateResumableDesired {
		t.Errorf("got recovery state %s, want %s", res.RecoveryState, RecoveryStateResumableDesired)
	}
	if res.SyncEligible {
		t.Errorf("expected not sync eligible")
	}
}

// TestFixture7_PreparedTransaction_ForeignBytes_BlockedConflict verifies:
// 7. Prepared transaction + foreign bytes -> blocked_conflict.
func TestFixture7_PreparedTransaction_ForeignBytes_BlockedConflict(t *testing.T) {
	home := t.TempDir()
	b1Bytes := []byte("# sdd-archive skill v1\n")
	b2Bytes := []byte("# sdd-archive skill v2\n")
	catalogB2 := makeTestCatalog("2.2.0", b2Bytes, 0644)

	filePath := filepath.Join(home, filepath.FromSlash(DefaultArchiveSkillRelPath))
	if err := os.MkdirAll(filepath.Dir(filePath), 0755); err != nil {
		t.Fatal(err)
	}
	// External modification during interrupted transaction
	if err := os.WriteFile(filePath, []byte("# conflict bytes\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(filePath, 0644); err != nil {
		t.Fatal(err)
	}

	sum1 := sha256.Sum256(b1Bytes)
	sha1 := "sha256:" + hex.EncodeToString(sum1[:])
	sum2 := sha256.Sum256(b2Bytes)
	sha2 := "sha256:" + hex.EncodeToString(sum2[:])

	manifest := ManagedBundleManifest{
		Schema:        ManifestSchemaV1,
		Generation:    1,
		TransactionID: "tx-1",
		Producer: ProducerInfo{
			Version:       "2.1.10",
			CatalogDigest: "sha256:old",
		},
		Resources: []ResourceEntry{
			{
				ResourceID:     DefaultArchiveSkillResourceID,
				TargetIdentity: TargetIdentityToken(DefaultArchiveSkillRelPath),
				CanonicalPath:  DefaultArchiveSkillRelPath,
				Ownership:      OwnershipDescriptor{Kind: OwnershipFullFile},
				DesiredSHA256:  sha1,
				ObservedSHA256: sha1,
				Mode:           0644,
				TransactionID:  "tx-1",
			},
		},
	}
	if err := WriteManifest(home, manifest); err != nil {
		t.Fatal(err)
	}

	journal := MigrationJournal{
		Schema:             JournalSchemaV1,
		TransactionID:      "tx-2",
		ExpectedGeneration: 1,
		ProposedGeneration: 2,
		LastPhase:          PhaseApplying,
		Resources: []ResourceJournalEntry{
			{
				ResourceID:    DefaultArchiveSkillResourceID,
				CanonicalPath: DefaultArchiveSkillRelPath,
				Ownership:     OwnershipDescriptor{Kind: OwnershipFullFile},
				BeforeSHA256:  sha1,
				BeforeMode:    0644,
				DesiredSHA256: sha2,
				DesiredMode:   0644,
			},
		},
	}
	if err := WriteJournal(home, journal); err != nil {
		t.Fatal(err)
	}

	res := Classify(context.Background(), home, catalogB2)
	if res.RecoveryState != RecoveryStateBlockedConflict {
		t.Errorf("got recovery state %s, want %s", res.RecoveryState, RecoveryStateBlockedConflict)
	}
	if res.SyncEligible {
		t.Errorf("expected not sync eligible")
	}
}

// TestFixture8_MixedResourceTransactions_Mixed verifies:
// 8. Manifest with mixed resource transaction IDs -> mixed, not eligible.
func TestFixture8_MixedResourceTransactions_Mixed(t *testing.T) {
	home := t.TempDir()
	b1Bytes := []byte("# sdd-archive skill v1\n")
	catalogB2 := makeTestCatalog("2.2.0", []byte("# sdd-archive skill v2\n"), 0644)

	filePath := filepath.Join(home, filepath.FromSlash(DefaultArchiveSkillRelPath))
	if err := os.MkdirAll(filepath.Dir(filePath), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filePath, b1Bytes, 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(filePath, 0644); err != nil {
		t.Fatal(err)
	}

	sum1 := sha256.Sum256(b1Bytes)
	sha1 := "sha256:" + hex.EncodeToString(sum1[:])

	manifest := ManagedBundleManifest{
		Schema:        ManifestSchemaV1,
		Generation:    1,
		TransactionID: "tx-1",
		Producer: ProducerInfo{
			Version:       "2.1.10",
			CatalogDigest: "sha256:old",
		},
		Resources: []ResourceEntry{
			{
				ResourceID:     DefaultArchiveSkillResourceID,
				TargetIdentity: TargetIdentityToken(DefaultArchiveSkillRelPath),
				CanonicalPath:  DefaultArchiveSkillRelPath,
				Ownership:      OwnershipDescriptor{Kind: OwnershipFullFile},
				DesiredSHA256:  sha1,
				ObservedSHA256: sha1,
				Mode:           0644,
				TransactionID:  "tx-foreign", // Mismatch with manifest.TransactionID
			},
		},
	}
	if err := WriteManifest(home, manifest); err != nil {
		t.Fatal(err)
	}

	res := Classify(context.Background(), home, catalogB2)
	if res.BundleIdentity != BundleIdentityMixed {
		t.Errorf("got bundle identity %s, want %s", res.BundleIdentity, BundleIdentityMixed)
	}
	if res.SyncEligible {
		t.Errorf("expected not sync eligible")
	}
}

// TestFixture9_NewerManifestSchema_UnsupportedNewerSchema verifies:
// 9. Newer manifest schema under older reader -> typed read-only refusal.
func TestFixture9_NewerManifestSchema_UnsupportedNewerSchema(t *testing.T) {
	home := t.TempDir()
	catalogB2 := makeTestCatalog("2.2.0", []byte("# sdd-archive skill v2\n"), 0644)

	manifestData := []byte(`{
  "schema": "gentle-ai.managed-bundle/v99",
  "generation": 1,
  "transaction_id": "tx-1",
  "producer": {
    "version": "99.0.0",
    "catalog_digest": "sha256:future"
  },
  "resources": []
}`)
	dir := filepath.Join(home, ManagedDirName, ManagedSubDir)
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ManifestFileName), manifestData, 0644); err != nil {
		t.Fatal(err)
	}

	res := Classify(context.Background(), home, catalogB2)
	if !res.UnsupportedSchema {
		t.Errorf("expected unsupported schema flag to be true")
	}
	if res.SyncEligible {
		t.Errorf("expected not sync eligible")
	}
}

// TestFixture10_SecondDoctorRun_ByteIdentical_ZeroWrites verifies:
// 10. Second doctor run is byte-identical and performs zero writes.
func TestFixture10_SecondDoctorRun_ByteIdentical_ZeroWrites(t *testing.T) {
	home := t.TempDir()
	b2Bytes := []byte("# sdd-archive skill v2\n")
	b2Mode := uint32(0644)
	catalogB2 := makeTestCatalog("2.2.0", b2Bytes, b2Mode)
	b2Digest, err := catalogB2.ComputeCatalogDigest()
	if err != nil {
		t.Fatalf("compute b2 digest: %v", err)
	}

	filePath := filepath.Join(home, filepath.FromSlash(DefaultArchiveSkillRelPath))
	if err := os.MkdirAll(filepath.Dir(filePath), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filePath, b2Bytes, 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(filePath, 0644); err != nil {
		t.Fatal(err)
	}

	sum := sha256.Sum256(b2Bytes)
	sha := "sha256:" + hex.EncodeToString(sum[:])

	manifest := ManagedBundleManifest{
		Schema:        ManifestSchemaV1,
		Generation:    2,
		TransactionID: "tx-2",
		Producer: ProducerInfo{
			Version:       "2.2.0",
			VCSRevision:   "commit-2.2.0",
			CatalogDigest: b2Digest,
		},
		Resources: []ResourceEntry{
			{
				ResourceID:     DefaultArchiveSkillResourceID,
				TargetIdentity: TargetIdentityToken(DefaultArchiveSkillRelPath),
				CanonicalPath:  DefaultArchiveSkillRelPath,
				Ownership:      OwnershipDescriptor{Kind: OwnershipFullFile},
				DesiredSHA256:  sha,
				ObservedSHA256: sha,
				Mode:           b2Mode,
				TransactionID:  "tx-2",
			},
		},
	}
	if err := WriteManifest(home, manifest); err != nil {
		t.Fatal(err)
	}

	manifestPath := filepath.Join(home, ManagedDirName, ManagedSubDir, ManifestFileName)
	manifestStat1, err := os.Stat(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	fileStat1, err := os.Stat(filePath)
	if err != nil {
		t.Fatal(err)
	}

	res1 := Classify(context.Background(), home, catalogB2)
	res2 := Classify(context.Background(), home, catalogB2)

	res1JSON, _ := json.Marshal(res1)
	res2JSON, _ := json.Marshal(res2)

	if string(res1JSON) != string(res2JSON) {
		t.Errorf("second run output differs: run1=%s, run2=%s", string(res1JSON), string(res2JSON))
	}

	manifestStat2, _ := os.Stat(manifestPath)
	fileStat2, _ := os.Stat(filePath)

	if manifestStat1.ModTime() != manifestStat2.ModTime() {
		t.Errorf("manifest was mutated during read-only doctor classification")
	}
	if fileStat1.ModTime() != fileStat2.ModTime() {
		t.Errorf("target file was mutated during read-only doctor classification")
	}
}

// TestJournal_InvalidTransactionID_Rejected verifies that path traversal transaction IDs are rejected.
func TestJournal_InvalidTransactionID_Rejected(t *testing.T) {
	home := t.TempDir()
	badJournals := []string{"../escape", "foo/bar", "foo\\bar", "..", ".", ""}
	for _, id := range badJournals {
		j := MigrationJournal{
			Schema:        JournalSchemaV1,
			TransactionID: id,
		}
		if err := WriteJournal(home, j); err == nil {
			t.Errorf("expected error for transaction ID %q, got nil", id)
		}
	}
}

// TestJournal_EmptyResources_BlockedConflict verifies empty journal resources cannot pass as resumable.
func TestJournal_EmptyResources_BlockedConflict(t *testing.T) {
	home := t.TempDir()
	catalog := makeTestCatalog("2.2.0", []byte("content"), 0644)
	manifest := ManagedBundleManifest{
		Schema:        ManifestSchemaV1,
		Generation:    1,
		TransactionID: "tx-1",
		Producer: ProducerInfo{
			Version:       "2.2.0",
			CatalogDigest: "sha256:digest",
		},
		Resources: []ResourceEntry{
			{
				ResourceID:     DefaultArchiveSkillResourceID,
				TargetIdentity: TargetIdentityToken(DefaultArchiveSkillRelPath),
				CanonicalPath:  DefaultArchiveSkillRelPath,
				Ownership:      OwnershipDescriptor{Kind: OwnershipFullFile},
				DesiredSHA256:  "sha256:1",
				ObservedSHA256: "sha256:1",
				Mode:           0644,
				TransactionID:  "tx-1",
			},
		},
	}
	if err := WriteManifest(home, manifest); err != nil {
		t.Fatal(err)
	}

	journal := MigrationJournal{
		Schema:             JournalSchemaV1,
		TransactionID:      "tx-2",
		ExpectedGeneration: 1,
		ProposedGeneration: 2,
		LastPhase:          PhaseApplying,
		Resources:          []ResourceJournalEntry{},
	}
	if err := WriteJournal(home, journal); err != nil {
		t.Fatal(err)
	}

	res := Classify(context.Background(), home, catalog)
	if res.RecoveryState != RecoveryStateBlockedConflict {
		t.Errorf("got recovery state %s, want %s", res.RecoveryState, RecoveryStateBlockedConflict)
	}
}

// TestJournal_MalformedJournal_BlockedConflict verifies malformed journal is surfaced as a conflict.
func TestJournal_MalformedJournal_BlockedConflict(t *testing.T) {
	home := t.TempDir()
	catalog := makeTestCatalog("2.2.0", []byte("content"), 0644)
	manifest := ManagedBundleManifest{
		Schema:        ManifestSchemaV1,
		Generation:    1,
		TransactionID: "tx-1",
		Producer: ProducerInfo{
			Version:       "2.2.0",
			CatalogDigest: "sha256:digest",
		},
		Resources: []ResourceEntry{
			{
				ResourceID:     DefaultArchiveSkillResourceID,
				TargetIdentity: TargetIdentityToken(DefaultArchiveSkillRelPath),
				CanonicalPath:  DefaultArchiveSkillRelPath,
				Ownership:      OwnershipDescriptor{Kind: OwnershipFullFile},
				DesiredSHA256:  "sha256:1",
				ObservedSHA256: "sha256:1",
				Mode:           0644,
				TransactionID:  "tx-1",
			},
		},
	}
	if err := WriteManifest(home, manifest); err != nil {
		t.Fatal(err)
	}

	dir := filepath.Join(home, ManagedDirName, ManagedSubDir, JournalDirName, "tx-broken")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, JournalFileName), []byte("{ invalid json"), 0644); err != nil {
		t.Fatal(err)
	}

	res := Classify(context.Background(), home, catalog)
	if res.RecoveryState != RecoveryStateBlockedConflict {
		t.Errorf("got recovery state %s, want %s", res.RecoveryState, RecoveryStateBlockedConflict)
	}
}

// TestJournal_UnsupportedJournalSchema_BlockedConflict verifies unsupported journal schema is rejected as conflict.
func TestJournal_UnsupportedJournalSchema_BlockedConflict(t *testing.T) {
	home := t.TempDir()
	catalog := makeTestCatalog("2.2.0", []byte("content"), 0644)
	manifest := ManagedBundleManifest{
		Schema:        ManifestSchemaV1,
		Generation:    1,
		TransactionID: "tx-1",
		Producer: ProducerInfo{
			Version:       "2.2.0",
			CatalogDigest: "sha256:digest",
		},
		Resources: []ResourceEntry{
			{
				ResourceID:     DefaultArchiveSkillResourceID,
				TargetIdentity: TargetIdentityToken(DefaultArchiveSkillRelPath),
				CanonicalPath:  DefaultArchiveSkillRelPath,
				Ownership:      OwnershipDescriptor{Kind: OwnershipFullFile},
				DesiredSHA256:  "sha256:1",
				ObservedSHA256: "sha256:1",
				Mode:           0644,
				TransactionID:  "tx-1",
			},
		},
	}
	if err := WriteManifest(home, manifest); err != nil {
		t.Fatal(err)
	}

	journal := MigrationJournal{
		Schema:             "gentle-ai.managed-journal/v999",
		TransactionID:      "tx-future",
		ExpectedGeneration: 1,
		ProposedGeneration: 2,
		LastPhase:          PhaseCompleted, // Looks completed, but schema is unsupported
		Resources: []ResourceJournalEntry{
			{
				ResourceID:    DefaultArchiveSkillResourceID,
				CanonicalPath: DefaultArchiveSkillRelPath,
			},
		},
	}
	dir := filepath.Join(home, ManagedDirName, ManagedSubDir, JournalDirName, "tx-future")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(journal)
	if err != nil {
		t.Fatalf("marshal journal: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, JournalFileName), data, 0644); err != nil {
		t.Fatal(err)
	}

	res := Classify(context.Background(), home, catalog)
	if res.RecoveryState != RecoveryStateBlockedConflict {
		t.Errorf("got recovery state %s, want %s", res.RecoveryState, RecoveryStateBlockedConflict)
	}
}

// TestJournal_DesiredModeMismatch_BlockedConflict verifies mode mismatch prevents false resumable_desired.
func TestJournal_DesiredModeMismatch_BlockedConflict(t *testing.T) {
	home := t.TempDir()
	content := []byte("# content\n")
	catalog := makeTestCatalog("2.2.0", content, 0644)

	filePath := filepath.Join(home, filepath.FromSlash(DefaultArchiveSkillRelPath))
	if err := os.MkdirAll(filepath.Dir(filePath), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filePath, content, 0600); err != nil { // mode is 0600, desired is 0644
		t.Fatal(err)
	}
	if err := os.Chmod(filePath, 0600); err != nil {
		t.Fatal(err)
	}

	sum := sha256.Sum256(content)
	sha := "sha256:" + hex.EncodeToString(sum[:])

	manifest := ManagedBundleManifest{
		Schema:        ManifestSchemaV1,
		Generation:    1,
		TransactionID: "tx-1",
		Producer: ProducerInfo{
			Version:       "2.1.10",
			CatalogDigest: "sha256:old",
		},
		Resources: []ResourceEntry{
			{
				ResourceID:     DefaultArchiveSkillResourceID,
				TargetIdentity: TargetIdentityToken(DefaultArchiveSkillRelPath),
				CanonicalPath:  DefaultArchiveSkillRelPath,
				Ownership:      OwnershipDescriptor{Kind: OwnershipFullFile},
				DesiredSHA256:  sha,
				ObservedSHA256: sha,
				Mode:           0644,
				TransactionID:  "tx-1",
			},
		},
	}
	if err := WriteManifest(home, manifest); err != nil {
		t.Fatal(err)
	}

	journal := MigrationJournal{
		Schema:             JournalSchemaV1,
		TransactionID:      "tx-2",
		ExpectedGeneration: 1,
		ProposedGeneration: 2,
		LastPhase:          PhaseApplying,
		Resources: []ResourceJournalEntry{
			{
				ResourceID:    DefaultArchiveSkillResourceID,
				CanonicalPath: DefaultArchiveSkillRelPath,
				Ownership:     OwnershipDescriptor{Kind: OwnershipFullFile},
				BeforeSHA256:  "sha256:before",
				BeforeMode:    0644,
				DesiredSHA256: sha,
				DesiredMode:   0644, // Desired mode is 0644, file on disk is 0600
			},
		},
	}
	if err := WriteJournal(home, journal); err != nil {
		t.Fatal(err)
	}

	res := Classify(context.Background(), home, catalog)
	if res.RecoveryState != RecoveryStateBlockedConflict {
		t.Errorf("got recovery state %s, want %s", res.RecoveryState, RecoveryStateBlockedConflict)
	}
}

// TestClassify_NonRegularTarget_TypeChanged verifies symlink/directory target is classified as type_changed.
func TestClassify_NonRegularTarget_TypeChanged(t *testing.T) {
	home := t.TempDir()
	content := []byte("# content\n")
	catalog := makeTestCatalog("2.2.0", content, 0644)
	digest, _ := catalog.ComputeCatalogDigest()

	filePath := filepath.Join(home, filepath.FromSlash(DefaultArchiveSkillRelPath))
	if err := os.MkdirAll(filePath, 0755); err != nil { // Directory instead of regular file!
		t.Fatal(err)
	}

	manifest := ManagedBundleManifest{
		Schema:        ManifestSchemaV1,
		Generation:    1,
		TransactionID: "tx-1",
		Producer: ProducerInfo{
			Version:       "2.2.0",
			CatalogDigest: digest,
		},
		Resources: []ResourceEntry{
			{
				ResourceID:     DefaultArchiveSkillResourceID,
				TargetIdentity: TargetIdentityToken(DefaultArchiveSkillRelPath),
				CanonicalPath:  DefaultArchiveSkillRelPath,
				Ownership:      OwnershipDescriptor{Kind: OwnershipFullFile},
				DesiredSHA256:  "sha256:1",
				ObservedSHA256: "sha256:1",
				Mode:           0644,
				TransactionID:  "tx-1",
			},
		},
	}
	if err := WriteManifest(home, manifest); err != nil {
		t.Fatal(err)
	}

	res := Classify(context.Background(), home, catalog)
	if res.ExtentIntegrity != ExtentIntegrityTypeChanged {
		t.Errorf("got extent integrity %s, want %s", res.ExtentIntegrity, ExtentIntegrityTypeChanged)
	}
}

// TestClassify_ResourceDescriptorMismatch_NotAligned verifies that manifest with incorrect descriptor details is not reported as aligned.
func TestClassify_ResourceDescriptorMismatch_NotAligned(t *testing.T) {
	home := t.TempDir()
	content := []byte("# content\n")
	catalog := makeTestCatalog("2.2.0", content, 0644)
	digest, _ := catalog.ComputeCatalogDigest()

	filePath := filepath.Join(home, filepath.FromSlash(DefaultArchiveSkillRelPath))
	if err := os.MkdirAll(filepath.Dir(filePath), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filePath, content, 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(filePath, 0644); err != nil {
		t.Fatal(err)
	}

	sum := sha256.Sum256(content)
	sha := "sha256:" + hex.EncodeToString(sum[:])

	// Manifest records matching Producer.CatalogDigest but has a decoy resource path
	manifest := ManagedBundleManifest{
		Schema:        ManifestSchemaV1,
		Generation:    1,
		TransactionID: "tx-1",
		Producer: ProducerInfo{
			Version:       "2.2.0",
			CatalogDigest: digest,
		},
		Resources: []ResourceEntry{
			{
				ResourceID:     DefaultArchiveSkillResourceID,
				TargetIdentity: TargetIdentityToken("other/decoy/path.md"),
				CanonicalPath:  DefaultArchiveSkillRelPath,
				Ownership:      OwnershipDescriptor{Kind: OwnershipFullFile},
				DesiredSHA256:  sha,
				ObservedSHA256: sha,
				Mode:           0644,
				TransactionID:  "tx-1",
			},
		},
	}
	if err := WriteManifest(home, manifest); err != nil {
		t.Fatal(err)
	}

	res := Classify(context.Background(), home, catalog)
	if res.BundleIdentity == BundleIdentityAligned {
		t.Errorf("expected mismatch to prevent aligned bundle identity, got %s", res.BundleIdentity)
	}
}

// TestClassify_PathTraversal_Rejected verifies that path traversal canonical paths are rejected.
func TestClassify_PathTraversal_Rejected(t *testing.T) {
	home := t.TempDir()
	catalog := makeTestCatalog("2.2.0", []byte("content"), 0644)

	manifest := ManagedBundleManifest{
		Schema:        ManifestSchemaV1,
		Generation:    1,
		TransactionID: "tx-1",
		Producer: ProducerInfo{
			Version:       "2.2.0",
			CatalogDigest: "sha256:digest",
		},
		Resources: []ResourceEntry{
			{
				ResourceID:     DefaultArchiveSkillResourceID,
				TargetIdentity: TargetIdentityToken("escape"),
				CanonicalPath:  "../../etc/passwd",
				Ownership:      OwnershipDescriptor{Kind: OwnershipFullFile},
				DesiredSHA256:  "sha256:1",
				ObservedSHA256: "sha256:1",
				Mode:           0644,
				TransactionID:  "tx-1",
			},
		},
	}
	if err := WriteManifest(home, manifest); err != nil {
		t.Fatal(err)
	}

	res := Classify(context.Background(), home, catalog)
	if res.ExtentIntegrity != ExtentIntegrityTypeChanged || res.SyncEligibleReason != SyncReasonTypeChanged {
		t.Errorf("expected path traversal to be rejected as type_changed, got %s (%s)", res.ExtentIntegrity, res.SyncEligibleReason)
	}
}

// TestClassify_ProducerVersionMismatch_Stale verifies that version difference prevents aligned state.
func TestClassify_ProducerVersionMismatch_Stale(t *testing.T) {
	home := t.TempDir()
	content := []byte("# content\n")
	catalogB2 := makeTestCatalog("2.2.0", content, 0644)
	digest, _ := catalogB2.ComputeCatalogDigest()

	filePath := filepath.Join(home, filepath.FromSlash(DefaultArchiveSkillRelPath))
	if err := os.MkdirAll(filepath.Dir(filePath), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filePath, content, 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(filePath, 0644); err != nil {
		t.Fatal(err)
	}

	sum := sha256.Sum256(content)
	sha := "sha256:" + hex.EncodeToString(sum[:])

	// Same digest and same VCSRevision but older version in producer info to isolate version mismatch
	manifest := ManagedBundleManifest{
		Schema:        ManifestSchemaV1,
		Generation:    1,
		TransactionID: "tx-1",
		Producer: ProducerInfo{
			Version:       "2.1.10",
			VCSRevision:   catalogB2.VCSRevision,
			CatalogDigest: digest,
		},
		Resources: []ResourceEntry{
			{
				ResourceID:     DefaultArchiveSkillResourceID,
				TargetIdentity: TargetIdentityToken(DefaultArchiveSkillRelPath),
				CanonicalPath:  DefaultArchiveSkillRelPath,
				Ownership:      OwnershipDescriptor{Kind: OwnershipFullFile},
				DesiredSHA256:  sha,
				ObservedSHA256: sha,
				Mode:           0644,
				TransactionID:  "tx-1",
			},
		},
	}
	if err := WriteManifest(home, manifest); err != nil {
		t.Fatal(err)
	}

	res := Classify(context.Background(), home, catalogB2)
	if res.BundleIdentity != BundleIdentityStale {
		t.Errorf("expected stale due to version mismatch, got %s", res.BundleIdentity)
	}
}

// TestJournal_MultipleJournals_ConflictPrecedence verifies that conflict takes precedence when multiple journals exist.
func TestJournal_MultipleJournals_ConflictPrecedence(t *testing.T) {
	home := t.TempDir()
	content := []byte("# content\n")
	catalog := makeTestCatalog("2.2.0", content, 0644)
	sum := sha256.Sum256(content)
	sha := "sha256:" + hex.EncodeToString(sum[:])

	filePath := filepath.Join(home, filepath.FromSlash(DefaultArchiveSkillRelPath))
	if err := os.MkdirAll(filepath.Dir(filePath), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filePath, content, 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(filePath, 0644); err != nil {
		t.Fatal(err)
	}

	manifest := ManagedBundleManifest{
		Schema:        ManifestSchemaV1,
		Generation:    1,
		TransactionID: "tx-1",
		Producer: ProducerInfo{
			Version:       "2.1.10",
			CatalogDigest: "sha256:old",
		},
		Resources: []ResourceEntry{
			{
				ResourceID:     DefaultArchiveSkillResourceID,
				TargetIdentity: TargetIdentityToken(DefaultArchiveSkillRelPath),
				CanonicalPath:  DefaultArchiveSkillRelPath,
				Ownership:      OwnershipDescriptor{Kind: OwnershipFullFile},
				DesiredSHA256:  sha,
				ObservedSHA256: sha,
				Mode:           0644,
				TransactionID:  "tx-1",
			},
		},
	}
	if err := WriteManifest(home, manifest); err != nil {
		t.Fatal(err)
	}

	// Journal 1: resumable
	journal1 := MigrationJournal{
		Schema:             JournalSchemaV1,
		TransactionID:      "tx-resumable",
		ExpectedGeneration: 1,
		ProposedGeneration: 2,
		LastPhase:          PhaseApplying,
		Resources: []ResourceJournalEntry{
			{
				ResourceID:    DefaultArchiveSkillResourceID,
				CanonicalPath: DefaultArchiveSkillRelPath,
				Ownership:     OwnershipDescriptor{Kind: OwnershipFullFile},
				BeforeSHA256:  "sha256:old",
				DesiredSHA256: sha,
				DesiredMode:   0644,
			},
		},
	}
	if err := WriteJournal(home, journal1); err != nil {
		t.Fatal(err)
	}

	// Journal 2: conflict
	journal2 := MigrationJournal{
		Schema:             JournalSchemaV1,
		TransactionID:      "tx-conflict",
		ExpectedGeneration: 1,
		ProposedGeneration: 2,
		LastPhase:          PhaseApplying,
		Resources: []ResourceJournalEntry{
			{
				ResourceID:    DefaultArchiveSkillResourceID,
				CanonicalPath: DefaultArchiveSkillRelPath,
				Ownership:     OwnershipDescriptor{Kind: OwnershipFullFile},
				BeforeSHA256:  "sha256:foreign",
				DesiredSHA256: "sha256:foreign2",
				DesiredMode:   0644,
			},
		},
	}
	if err := WriteJournal(home, journal2); err != nil {
		t.Fatal(err)
	}

	res := Classify(context.Background(), home, catalog)
	if res.RecoveryState != RecoveryStateBlockedConflict {
		t.Errorf("expected RecoveryStateBlockedConflict when a conflicting journal exists, got %s", res.RecoveryState)
	}
}

// TestClassify_EmptyDesiredExtentSHA_FallbackCalculated verifies that empty DesiredExtent.SHA256 falls back to calculated content hash.
func TestClassify_EmptyDesiredExtentSHA_FallbackCalculated(t *testing.T) {
	home := t.TempDir()
	content := []byte("# content\n")
	sum := sha256.Sum256(content)
	sha := "sha256:" + hex.EncodeToString(sum[:])

	catalog := ResourceCatalog{
		Version:     "2.2.0",
		VCSRevision: "commit-2.2.0",
		Resources: []ResourceDescriptor{
			{
				ResourceID:    DefaultArchiveSkillResourceID,
				SchemaVersion: 1,
				ComponentID:   model.ComponentSkills,
				AgentID:       model.AgentOpenCode,
				CanonicalPath: DefaultArchiveSkillRelPath,
				Ownership:     OwnershipDescriptor{Kind: OwnershipFullFile},
				RenderDesired: func() (DesiredExtent, error) {
					return DesiredExtent{
						Content: content,
						SHA256:  "", // Empty Desired SHA256!
						Mode:    0644,
					}, nil
				},
			},
		},
	}
	digest, err := catalog.ComputeCatalogDigest()
	if err != nil {
		t.Fatal(err)
	}

	filePath := filepath.Join(home, filepath.FromSlash(DefaultArchiveSkillRelPath))
	if err := os.MkdirAll(filepath.Dir(filePath), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filePath, content, 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(filePath, 0644); err != nil {
		t.Fatal(err)
	}

	manifest := ManagedBundleManifest{
		Schema:        ManifestSchemaV1,
		Generation:    1,
		TransactionID: "tx-1",
		Producer: ProducerInfo{
			Version:       "2.2.0",
			VCSRevision:   "commit-2.2.0",
			CatalogDigest: digest,
		},
		Resources: []ResourceEntry{
			{
				ResourceID:     DefaultArchiveSkillResourceID,
				TargetIdentity: TargetIdentityToken(DefaultArchiveSkillRelPath),
				CanonicalPath:  DefaultArchiveSkillRelPath,
				Ownership:      OwnershipDescriptor{Kind: OwnershipFullFile},
				DesiredSHA256:  sha,
				ObservedSHA256: sha,
				Mode:           0644,
				TransactionID:  "tx-1",
			},
		},
	}
	if err := WriteManifest(home, manifest); err != nil {
		t.Fatal(err)
	}

	res := Classify(context.Background(), home, catalog)
	if res.BundleIdentity != BundleIdentityAligned {
		t.Errorf("expected aligned bundle identity with fallback sha calculation, got %s (%s)", res.BundleIdentity, res.Detail)
	}
}
