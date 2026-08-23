package managedbundle

import (
	"os"
	"path/filepath"
	"testing"
)

func TestTransactionPublishesEvidenceDoctorConsumes(t *testing.T) {
	home := t.TempDir()
	descriptor := Descriptor{ResourceID: ArchiveSkillResourceID, CanonicalPath: ArchiveSkillTargetPath, Ownership: OwnershipFullFile, Content: []byte("desired"), Mode: 0o644}
	writeTarget(t, home, descriptor, []byte("before"), 0o600)
	writeCommittedBefore(t, home, descriptor, []byte("before"), 0o600)
	transaction, err := Prepare(home, []Descriptor{descriptor})
	if err != nil {
		t.Fatal(err)
	}
	prepared := ClassifyExtents(home, "current", "", []Descriptor{descriptor})
	if prepared.RecoveryState != RecoveryResumableBefore {
		t.Fatalf("prepared classification = %#v", prepared)
	}
	writeTarget(t, home, descriptor, descriptor.Content, os.FileMode(descriptor.Mode))
	applied := ClassifyExtents(home, "current", "", []Descriptor{descriptor})
	if applied.RecoveryState != RecoveryResumableDesired {
		t.Fatalf("applied classification = %#v", applied)
	}
	if err := transaction.VerifyAndCommit("current", "revision"); err != nil {
		t.Fatal(err)
	}
	committed := ClassifyExtents(home, "current", "revision", []Descriptor{descriptor})
	if committed.BundleIdentity != BundleAligned || committed.ExtentIntegrity != ExtentMatch || committed.RecoveryState != "" && committed.RecoveryState != RecoveryNone {
		t.Fatalf("committed classification = %#v", committed)
	}
}

func TestTransactionRestartDoesNotReapplyOrCommitForeignBytes(t *testing.T) {
	home := t.TempDir()
	descriptor := Descriptor{ResourceID: ArchiveSkillResourceID, CanonicalPath: ArchiveSkillTargetPath, Ownership: OwnershipFullFile, Content: []byte("desired"), Mode: 0o644}
	writeTarget(t, home, descriptor, []byte("before"), 0o600)
	writeCommittedBefore(t, home, descriptor, []byte("before"), 0o600)
	transaction, err := Prepare(home, []Descriptor{descriptor})
	if err != nil {
		t.Fatal(err)
	}
	writeTarget(t, home, descriptor, []byte("foreign"), 0o644)
	if got := ClassifyExtents(home, "current", "", []Descriptor{descriptor}); got.RecoveryState != RecoveryBlocked {
		t.Fatalf("foreign classification = %#v", got)
	}
	if err := transaction.VerifyAndCommit("current", ""); err == nil {
		t.Fatal("commit succeeded with foreign bytes")
	}
	manifest, err := readManifest(home)
	if err != nil || manifest.TransactionID != "tx-old" || manifest.Generation != 1 {
		t.Fatalf("failed verification changed committed manifest: %#v, %v", manifest, err)
	}
	journalPath := filepath.Join(home, filepath.FromSlash(JournalDir), transaction.journal.TransactionID, "journal.json")
	if _, err := os.Stat(journalPath); err != nil {
		t.Fatalf("prepared journal was lost: %v", err)
	}
}

func TestTransactionRejectsUncommittedExistingFile(t *testing.T) {
	home := t.TempDir()
	descriptor := Descriptor{ResourceID: ArchiveSkillResourceID, CanonicalPath: ArchiveSkillTargetPath, Ownership: OwnershipFullFile, Content: []byte("desired"), Mode: 0o644}
	writeTarget(t, home, descriptor, []byte("legacy-or-user"), 0o644)
	if _, err := Prepare(home, []Descriptor{descriptor}); err == nil {
		t.Fatal("prepared overwrite without committed ownership evidence")
	}
	if _, err := os.Stat(filepath.Join(home, filepath.FromSlash(JournalDir))); !os.IsNotExist(err) {
		t.Fatalf("journal created for untrusted file: %v", err)
	}
}

func writeCommittedBefore(t *testing.T, home string, descriptor Descriptor, content []byte, mode uint32) {
	t.Helper()
	manifest := Manifest{Schema: ManifestSchemaV1, Generation: 1, TransactionID: "tx-old", Producer: Producer{Version: "old", CatalogDigest: "sha256:old"}, Resources: []Resource{{ResourceID: descriptor.ResourceID, CanonicalPath: descriptor.CanonicalPath, Ownership: descriptor.Ownership, DesiredSHA256: SHA256(content), ObservedSHA256: SHA256(content), Mode: mode}}}
	writeManifestFixture(t, home, manifest)
}
