package managedbundle

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestClassifyRecoveryStates(t *testing.T) {
	descriptor := Descriptor{ResourceID: ArchiveSkillResourceID, CanonicalPath: ArchiveSkillTargetPath, Ownership: OwnershipFullFile, Content: []byte("desired"), Mode: 0o644}
	digest, err := Digest([]Descriptor{descriptor})
	if err != nil {
		t.Fatal(err)
	}
	manifest := testManifest(descriptor, digest, "current")
	before, desired := []byte("before"), descriptor.Content
	tests := []struct {
		name    string
		content []byte
		mode    os.FileMode
		mutate  func(*Journal)
		want    RecoveryState
	}{
		{name: "observed before", content: before, mode: 0o600, want: RecoveryResumableBefore},
		{name: "observed desired", content: desired, mode: 0o644, want: RecoveryResumableDesired},
		{name: "foreign bytes", content: []byte("foreign"), mode: 0o644, want: RecoveryBlocked},
		{name: "desired mode missing", content: desired, mode: 0o600, want: RecoveryBlocked},
		{name: "unsupported schema", content: before, mode: 0o600, mutate: func(j *Journal) { j.Schema = "v99" }, want: RecoveryBlocked},
		{name: "empty resources", content: before, mode: 0o600, mutate: func(j *Journal) { j.Resources = nil }, want: RecoveryBlocked},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			home := t.TempDir()
			writeManifestFixture(t, home, manifest)
			writeTarget(t, home, descriptor, tt.content, tt.mode)
			journal := recoveryJournal(descriptor, before)
			if tt.mutate != nil {
				tt.mutate(&journal)
			}
			writeJournalFixture(t, home, journal)
			got := ClassifyExtents(home, "current", "", []Descriptor{descriptor})
			if got.RecoveryState != tt.want || got.SyncEligible || got.ExtentIntegrity == ExtentMatch {
				t.Fatalf("classification = %#v", got)
			}
		})
	}
}

func TestClassifyRecoveryRejectsUnsafeAndMultipleTransactions(t *testing.T) {
	descriptor := Descriptor{ResourceID: ArchiveSkillResourceID, CanonicalPath: ArchiveSkillTargetPath, Ownership: OwnershipFullFile, Content: []byte("desired"), Mode: 0o644}
	digest, err := Digest([]Descriptor{descriptor})
	if err != nil {
		t.Fatal(err)
	}
	home := t.TempDir()
	writeManifestFixture(t, home, testManifest(descriptor, digest, "current"))
	writeTarget(t, home, descriptor, []byte("before"), 0o600)
	first := recoveryJournal(descriptor, []byte("before"))
	writeJournalFixture(t, home, first)
	second := first
	second.TransactionID = "tx-2"
	second.Resources[0].BeforeSHA256 = SHA256([]byte("other"))
	writeJournalFixture(t, home, second)
	got := ClassifyExtents(home, "current", "", []Descriptor{descriptor})
	if got.RecoveryState != RecoveryBlocked {
		t.Fatalf("classification = %#v", got)
	}
}

func recoveryJournal(descriptor Descriptor, before []byte) Journal {
	return Journal{Schema: JournalSchemaV1, TransactionID: "tx-2", ExpectedGeneration: 1, ProposedGeneration: 2, Phase: "prepared", Resources: []JournalResource{{ResourceID: descriptor.ResourceID, CanonicalPath: descriptor.CanonicalPath, BeforeSHA256: SHA256(before), BeforeMode: 0o600, DesiredSHA256: SHA256(descriptor.Content), DesiredMode: descriptor.Mode}}}
}

func writeJournalFixture(t *testing.T, home string, journal Journal) {
	t.Helper()
	data, err := json.MarshalIndent(journal, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(home, filepath.FromSlash(JournalDir), journal.TransactionID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "journal.json"), append(data, '\n'), 0o600); err != nil {
		t.Fatal(err)
	}
}
