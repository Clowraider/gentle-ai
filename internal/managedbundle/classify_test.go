package managedbundle

import (
	"os"
	"path/filepath"
	"testing"
)

func TestClassifyExtentsInspectsDiskBeforeClaimingMatch(t *testing.T) {
	descriptor := Descriptor{ResourceID: ArchiveSkillResourceID, CanonicalPath: ArchiveSkillTargetPath, Ownership: OwnershipFullFile, Content: []byte("desired"), Mode: 0o644}
	digest, err := Digest([]Descriptor{descriptor})
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name      string
		manifest  Manifest
		prepare   func(string)
		identity  BundleIdentity
		integrity ExtentIntegrity
		eligible  bool
	}{
		{name: "aligned", manifest: testManifest(descriptor, digest, "current"), prepare: func(home string) { writeTarget(t, home, descriptor, []byte("desired"), 0o644) }, identity: BundleAligned, integrity: ExtentMatch},
		{name: "stale", manifest: testManifest(descriptor, "sha256:older", "older"), prepare: func(home string) { writeTarget(t, home, descriptor, []byte("desired"), 0o644) }, identity: BundleStale, integrity: ExtentMatch, eligible: true},
		{name: "content changed", manifest: testManifest(descriptor, digest, "current"), prepare: func(home string) { writeTarget(t, home, descriptor, []byte("foreign"), 0o644) }, identity: BundleUserModified, integrity: ExtentUserModified},
		{name: "mode changed", manifest: testManifest(descriptor, digest, "current"), prepare: func(home string) { writeTarget(t, home, descriptor, []byte("desired"), 0o600) }, identity: BundleUserModified, integrity: ExtentUserModified},
		{name: "missing", manifest: testManifest(descriptor, digest, "current"), identity: BundleUserModified, integrity: ExtentMissing},
		{name: "directory", manifest: testManifest(descriptor, digest, "current"), prepare: func(home string) {
			if err := os.MkdirAll(ResolveTarget(home, descriptor), 0o755); err != nil {
				t.Fatal(err)
			}
		}, identity: BundleUserModified, integrity: ExtentTypeChanged},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			home := t.TempDir()
			writeManifestFixture(t, home, tt.manifest)
			if tt.prepare != nil {
				tt.prepare(home)
			}
			got := ClassifyExtents(home, "current", "", []Descriptor{descriptor})
			if got.BundleIdentity != tt.identity || got.ExtentIntegrity != tt.integrity || got.SyncEligible != tt.eligible {
				t.Fatalf("classification = %#v", got)
			}
		})
	}
}

func TestClassifyExtentsRejectsUntrustedInputs(t *testing.T) {
	descriptor := Descriptor{ResourceID: ArchiveSkillResourceID, CanonicalPath: ArchiveSkillTargetPath, Ownership: OwnershipFullFile, Content: []byte("desired"), Mode: 0o644}
	for _, name := range []string{"no manifest", "malformed", "unsupported", "path traversal", "symlink"} {
		t.Run(name, func(t *testing.T) {
			home := t.TempDir()
			switch name {
			case "malformed":
				writeRawManifest(t, home, []byte("{"))
			case "unsupported":
				writeManifestFixture(t, home, Manifest{Schema: "gentle-ai.managed-bundle/v99"})
			case "path traversal":
				bad := descriptor
				bad.CanonicalPath = "../escape"
				writeManifestFixture(t, home, testManifest(bad, "sha256:old", "old"))
			case "symlink":
				writeManifestFixture(t, home, testManifest(descriptor, "sha256:old", "old"))
				target := ResolveTarget(home, descriptor)
				if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(filepath.Join(home, "elsewhere"), target); err != nil {
					t.Fatal(err)
				}
			}
			got := ClassifyExtents(home, "current", "", []Descriptor{descriptor})
			if got.ExtentIntegrity == ExtentMatch || got.BundleIdentity == BundleAligned || got.SyncEligible {
				t.Fatalf("unsafe input classified as trusted: %#v", got)
			}
		})
	}
}

func testManifest(descriptor Descriptor, catalogDigest, version string) Manifest {
	digest := SHA256(descriptor.Content)
	return Manifest{Schema: ManifestSchemaV1, Generation: 1, TransactionID: "tx-1", Producer: Producer{Version: version, CatalogDigest: catalogDigest}, Resources: []Resource{{ResourceID: descriptor.ResourceID, CanonicalPath: descriptor.CanonicalPath, Ownership: descriptor.Ownership, DesiredSHA256: digest, ObservedSHA256: digest, Mode: descriptor.Mode}}}
}

func writeTarget(t *testing.T, home string, descriptor Descriptor, content []byte, mode os.FileMode) {
	t.Helper()
	target := ResolveTarget(home, descriptor)
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, content, mode); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(target, mode); err != nil {
		t.Fatal(err)
	}
}

func writeManifestFixture(t *testing.T, home string, manifest Manifest) {
	t.Helper()
	data, err := MarshalManifest(manifest)
	if err != nil {
		t.Fatal(err)
	}
	writeRawManifest(t, home, data)
}
func writeRawManifest(t *testing.T, home string, data []byte) {
	t.Helper()
	path := ManifestPath(home)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
}
