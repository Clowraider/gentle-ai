package managedbundle

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/gentleman-programming/gentle-ai/v2/internal/components/filemerge"
)

type Transaction struct {
	homeDir     string
	journal     Journal
	descriptors []Descriptor
}

func Prepare(homeDir string, descriptors []Descriptor) (*Transaction, error) {
	if len(descriptors) == 0 {
		return nil, errors.New("prepare managed bundle transaction: empty catalog")
	}
	manifest, err := readManifest(homeDir)
	manifestPresent := err == nil
	if errors.Is(err, os.ErrNotExist) || err != nil && !fileExists(ManifestPath(homeDir)) {
		manifest = Manifest{Schema: ManifestSchemaV1}
	} else if err != nil {
		return nil, err
	}
	committed := make(map[string]Resource, len(manifest.Resources))
	for _, resource := range manifest.Resources {
		committed[resource.ResourceID] = resource
	}
	id := fmt.Sprintf("tx-%d", time.Now().UTC().UnixNano())
	journal := Journal{Schema: JournalSchemaV1, TransactionID: id, ExpectedGeneration: manifest.Generation, ProposedGeneration: manifest.Generation + 1, Phase: "prepared"}
	for _, descriptor := range descriptors {
		target, err := safeTarget(homeDir, descriptor.CanonicalPath)
		if err != nil {
			return nil, err
		}
		resource := JournalResource{ResourceID: descriptor.ResourceID, CanonicalPath: descriptor.CanonicalPath, DesiredSHA256: SHA256(descriptor.Content), DesiredMode: descriptor.Mode}
		info, err := os.Lstat(target)
		switch {
		case errors.Is(err, os.ErrNotExist):
			resource.BeforeSHA256 = "missing"
		case err != nil:
			return nil, fmt.Errorf("inspect managed resource %s: %w", descriptor.ResourceID, err)
		case !info.Mode().IsRegular():
			return nil, fmt.Errorf("managed resource %s is not a regular file", descriptor.ResourceID)
		default:
			content, err := os.ReadFile(target)
			if err != nil {
				return nil, err
			}
			resource.BeforeSHA256, resource.BeforeMode = SHA256(content), uint32(info.Mode().Perm())
			previous, trusted := committed[descriptor.ResourceID]
			if !manifestPresent || !trusted || previous.CanonicalPath != descriptor.CanonicalPath || previous.Ownership != descriptor.Ownership || previous.ObservedSHA256 != resource.BeforeSHA256 || previous.Mode != resource.BeforeMode {
				return nil, fmt.Errorf("managed resource %s has no exact committed ownership evidence", descriptor.ResourceID)
			}
		}
		journal.Resources = append(journal.Resources, resource)
	}
	transaction := &Transaction{homeDir: homeDir, journal: journal, descriptors: append([]Descriptor(nil), descriptors...)}
	if err := transaction.writeJournal(); err != nil {
		return nil, err
	}
	return transaction, nil
}

func (transaction *Transaction) VerifyAndCommit(version, revision string) error {
	resources := make([]Resource, 0, len(transaction.descriptors))
	for _, descriptor := range transaction.descriptors {
		target := ResolveTarget(transaction.homeDir, descriptor)
		info, err := os.Lstat(target)
		if err != nil || !info.Mode().IsRegular() {
			return fmt.Errorf("verify managed resource %s: target is not a regular file", descriptor.ResourceID)
		}
		content, err := os.ReadFile(target)
		if err != nil {
			return err
		}
		digest := SHA256(content)
		if digest != SHA256(descriptor.Content) || uint32(info.Mode().Perm()) != descriptor.Mode {
			return fmt.Errorf("verify managed resource %s: desired extent not visible", descriptor.ResourceID)
		}
		resources = append(resources, Resource{ResourceID: descriptor.ResourceID, CanonicalPath: descriptor.CanonicalPath, Ownership: descriptor.Ownership, DesiredSHA256: digest, ObservedSHA256: digest, Mode: descriptor.Mode})
	}
	catalogDigest, err := Digest(transaction.descriptors)
	if err != nil {
		return err
	}
	manifest := Manifest{Schema: ManifestSchemaV1, Generation: transaction.journal.ProposedGeneration, TransactionID: transaction.journal.TransactionID, Producer: Producer{Version: version, VCSRevision: revision, CatalogDigest: catalogDigest}, Resources: resources}
	data, err := MarshalManifest(manifest)
	if err != nil {
		return err
	}
	if _, err := filemerge.WriteFileAtomic(ManifestPath(transaction.homeDir), data, 0o600); err != nil {
		return fmt.Errorf("commit managed bundle manifest: %w", err)
	}
	if committed, err := readManifest(transaction.homeDir); err != nil || committed.TransactionID != transaction.journal.TransactionID {
		return errors.New("commit managed bundle manifest: read-back mismatch")
	}
	transaction.journal.Phase = "completed"
	return transaction.writeJournal()
}

func (transaction *Transaction) writeJournal() error {
	data, err := json.MarshalIndent(transaction.journal, "", "  ")
	if err != nil {
		return err
	}
	path := filepath.Join(transaction.homeDir, filepath.FromSlash(JournalDir), transaction.journal.TransactionID, "journal.json")
	if _, err := filemerge.WriteFileAtomic(path, append(data, '\n'), 0o600); err != nil {
		return fmt.Errorf("write managed bundle journal: %w", err)
	}
	return nil
}

func fileExists(path string) bool {
	_, err := os.Lstat(path)
	return err == nil
}
