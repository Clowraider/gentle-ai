package managedbundle

import (
	"bytes"
	"path/filepath"
	"testing"
)

func TestDigestIsCanonicalAndContentBound(t *testing.T) {
	first := Descriptor{ResourceID: "b", CanonicalPath: "managed/b", Ownership: OwnershipFullFile, Content: []byte("b"), Mode: 0o644}
	second := Descriptor{ResourceID: "a", CanonicalPath: "managed/a", Ownership: OwnershipFullFile, Content: []byte("a"), Mode: 0o600}

	want, err := Digest([]Descriptor{first, second})
	if err != nil {
		t.Fatal(err)
	}
	got, err := Digest([]Descriptor{second, first})
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("digest depends on descriptor order: %q != %q", got, want)
	}

	first.Content = []byte("changed")
	changed, err := Digest([]Descriptor{first, second})
	if err != nil {
		t.Fatal(err)
	}
	if changed == want {
		t.Fatal("digest did not change with rendered owned extent")
	}
}

func TestCatalogUsesPortableCanonicalIdentity(t *testing.T) {
	catalog, err := Catalog()
	if err != nil {
		t.Fatal(err)
	}
	if len(catalog) != 1 {
		t.Fatalf("catalog resources = %d, want 1", len(catalog))
	}
	descriptor := catalog[0]
	if descriptor.ResourceID != ArchiveSkillResourceID || descriptor.CanonicalPath != ArchiveSkillTargetPath || descriptor.Ownership != OwnershipFullFile {
		t.Fatalf("descriptor = %#v", descriptor)
	}
	if filepath.IsAbs(descriptor.CanonicalPath) || len(descriptor.Content) == 0 || descriptor.Mode != 0o644 {
		t.Fatalf("descriptor is not portable and complete: %#v", descriptor)
	}
}

func TestMarshalManifestIsDeterministic(t *testing.T) {
	manifest := Manifest{
		Schema: ManifestSchemaV1, Generation: 2, TransactionID: "tx-2",
		Producer: Producer{Version: "2.2.0", VCSRevision: "abc", CatalogDigest: "sha256:catalog"},
		Resources: []Resource{
			{ResourceID: "b", CanonicalPath: "managed/b", Ownership: OwnershipFullFile, DesiredSHA256: "sha256:b", ObservedSHA256: "sha256:b", Mode: 0o644},
			{ResourceID: "a", CanonicalPath: "managed/a", Ownership: OwnershipFullFile, DesiredSHA256: "sha256:a", ObservedSHA256: "sha256:a", Mode: 0o600},
		},
	}
	first, err := MarshalManifest(manifest)
	if err != nil {
		t.Fatal(err)
	}
	second, err := MarshalManifest(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(first, second) || first[len(first)-1] != '\n' {
		t.Fatalf("serialization is not deterministic:\n%s\n%s", first, second)
	}
	if bytes.Index(first, []byte(`"resource_id": "a"`)) > bytes.Index(first, []byte(`"resource_id": "b"`)) {
		t.Fatalf("resources are not canonicalized:\n%s", first)
	}
}
