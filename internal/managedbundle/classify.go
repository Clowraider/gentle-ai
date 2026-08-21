package managedbundle

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func resolveSafeCanonicalPath(homeDir, relPath string) (string, error) {
	relPath = strings.TrimSpace(relPath)
	if relPath == "" {
		return "", errors.New("canonical path is empty")
	}
	if filepath.IsAbs(relPath) {
		return "", fmt.Errorf("canonical path %q must not be absolute", relPath)
	}
	joined := filepath.Join(homeDir, filepath.FromSlash(relPath))
	cleanHome := filepath.Clean(homeDir)
	rel, err := filepath.Rel(cleanHome, joined)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("canonical path %q escapes home directory", relPath)
	}
	return joined, nil
}

// Classify reads the durable managed bundle manifest and journals under homeDir,
// comparing them against the running catalog and actual files on disk.
func Classify(ctx context.Context, homeDir string, catalog ResourceCatalog) ClassificationResult {
	manifestPath := filepath.Join(homeDir, ManagedDirName, ManagedSubDir, ManifestFileName)
	manifestData, err := os.ReadFile(manifestPath)
	if err != nil {
		if os.IsNotExist(err) {
			return ClassificationResult{
				BundleIdentity:     BundleIdentityUnknown,
				ExtentIntegrity:    ExtentIntegrityMissing,
				RecoveryState:      RecoveryStateNone,
				SyncEligible:       false,
				SyncEligibleReason: SyncReasonNoManifest,
				Detail:             "no managed bundle manifest found — asset state unknown",
			}
		}
		return ClassificationResult{
			BundleIdentity:     BundleIdentityUnknown,
			ExtentIntegrity:    ExtentIntegrityMissing,
			RecoveryState:      RecoveryStateNone,
			SyncEligible:       false,
			SyncEligibleReason: SyncReasonManifestUnreadable,
			Detail:             fmt.Sprintf("failed to read managed bundle manifest: %v", err),
		}
	}

	var manifest ManagedBundleManifest
	if err := json.Unmarshal(manifestData, &manifest); err != nil {
		return ClassificationResult{
			BundleIdentity:     BundleIdentityUnknown,
			ExtentIntegrity:    ExtentIntegrityMissing,
			RecoveryState:      RecoveryStateNone,
			SyncEligible:       false,
			SyncEligibleReason: SyncReasonManifestMalformed,
			Detail:             "managed bundle manifest is malformed",
		}
	}

	if manifest.Schema != ManifestSchemaV1 {
		return ClassificationResult{
			BundleIdentity:     BundleIdentityUnknown,
			ExtentIntegrity:    ExtentIntegrityMissing,
			RecoveryState:      RecoveryStateNone,
			SyncEligible:       false,
			SyncEligibleReason: SyncReasonUnsupportedSchema,
			UnsupportedSchema:  true,
			Detail:             fmt.Sprintf("unsupported manifest schema %q", manifest.Schema),
		}
	}

	// Check if there are active / uncommitted transactions
	recoveryState, journalDetail := checkActiveTransactions(homeDir, manifest)
	if recoveryState != RecoveryStateNone {
		return ClassificationResult{
			BundleIdentity:     BundleIdentityUnknown,
			ExtentIntegrity:    ExtentIntegrityUserModified,
			RecoveryState:      recoveryState,
			SyncEligible:       false,
			SyncEligibleReason: SyncReasonUnresolvedTransaction,
			Detail:             journalDetail,
		}
	}

	// Validate resources in the manifest
	if len(manifest.Resources) == 0 {
		return ClassificationResult{
			BundleIdentity:     BundleIdentityUnknown,
			ExtentIntegrity:    ExtentIntegrityMissing,
			RecoveryState:      RecoveryStateNone,
			SyncEligible:       false,
			SyncEligibleReason: SyncReasonEmptyManifest,
			Detail:             "managed bundle manifest contains no resource entries",
		}
	}

	// Check for producer consistency
	for _, res := range manifest.Resources {
		if res.TransactionID != "" && manifest.TransactionID != "" && res.TransactionID != manifest.TransactionID {
			return ClassificationResult{
				BundleIdentity:     BundleIdentityMixed,
				ExtentIntegrity:    ExtentIntegrityUserModified,
				RecoveryState:      RecoveryStateNone,
				SyncEligible:       false,
				SyncEligibleReason: SyncReasonMixedResourceTransactions,
				Detail:             "managed bundle contains mixed resource transactions",
			}
		}
	}

	// Check on-disk extents for all committed resources
	for _, res := range manifest.Resources {
		targetPath, pathErr := resolveSafeCanonicalPath(homeDir, res.CanonicalPath)
		if pathErr != nil {
			return ClassificationResult{
				BundleIdentity:     BundleIdentityUserModified,
				ExtentIntegrity:    ExtentIntegrityTypeChanged,
				RecoveryState:      RecoveryStateNone,
				SyncEligible:       false,
				SyncEligibleReason: SyncReasonTypeChanged,
				Detail:             fmt.Sprintf("managed resource %s has invalid path: %v", res.ResourceID, pathErr),
			}
		}

		fi, err := os.Lstat(targetPath)
		if err != nil {
			if os.IsNotExist(err) {
				return ClassificationResult{
					BundleIdentity:     BundleIdentityUserModified,
					ExtentIntegrity:    ExtentIntegrityMissing,
					RecoveryState:      RecoveryStateNone,
					SyncEligible:       false,
					SyncEligibleReason: SyncReasonResourceMissing,
					Detail:             fmt.Sprintf("managed resource %s is missing from disk: %s", res.ResourceID, res.CanonicalPath),
				}
			}
			return ClassificationResult{
				BundleIdentity:     BundleIdentityUnknown,
				ExtentIntegrity:    ExtentIntegrityUserModified,
				RecoveryState:      RecoveryStateNone,
				SyncEligible:       false,
				SyncEligibleReason: SyncReasonResourceUnreadable,
				Detail:             fmt.Sprintf("managed resource %s cannot be read: %v", res.ResourceID, err),
			}
		}

		if !fi.Mode().IsRegular() {
			return ClassificationResult{
				BundleIdentity:     BundleIdentityUserModified,
				ExtentIntegrity:    ExtentIntegrityTypeChanged,
				RecoveryState:      RecoveryStateNone,
				SyncEligible:       false,
				SyncEligibleReason: SyncReasonTypeChanged,
				Detail:             fmt.Sprintf("managed resource %s type changed (not a regular file): %s", res.ResourceID, res.CanonicalPath),
			}
		}

		content, err := os.ReadFile(targetPath)
		if err != nil {
			return ClassificationResult{
				BundleIdentity:     BundleIdentityUnknown,
				ExtentIntegrity:    ExtentIntegrityUserModified,
				RecoveryState:      RecoveryStateNone,
				SyncEligible:       false,
				SyncEligibleReason: SyncReasonResourceUnreadable,
				Detail:             fmt.Sprintf("managed resource %s cannot be read: %v", res.ResourceID, err),
			}
		}

		sum := sha256.Sum256(content)
		observedSHA := "sha256:" + hex.EncodeToString(sum[:])
		if observedSHA != res.ObservedSHA256 {
			return ClassificationResult{
				BundleIdentity:     BundleIdentityUserModified,
				ExtentIntegrity:    ExtentIntegrityUserModified,
				RecoveryState:      RecoveryStateNone,
				SyncEligible:       false,
				SyncEligibleReason: SyncReasonContentMismatch,
				Detail:             fmt.Sprintf("managed resource %s was modified on disk", res.ResourceID),
			}
		}

		// Optional check: compare mode if res.Mode is specified
		if res.Mode != 0 {
			fileMode := uint32(fi.Mode().Perm())
			if fileMode != res.Mode {
				return ClassificationResult{
					BundleIdentity:     BundleIdentityUserModified,
					ExtentIntegrity:    ExtentIntegrityUserModified,
					RecoveryState:      RecoveryStateNone,
					SyncEligible:       false,
					SyncEligibleReason: SyncReasonModeMismatch,
					Detail:             fmt.Sprintf("managed resource %s file mode was modified on disk", res.ResourceID),
				}
			}
		}
	}

	// Compute current catalog digest
	currentCatalogDigest, err := catalog.ComputeCatalogDigest()
	if err != nil {
		return ClassificationResult{
			BundleIdentity:     BundleIdentityUnknown,
			ExtentIntegrity:    ExtentIntegrityMatch,
			RecoveryState:      RecoveryStateNone,
			SyncEligible:       false,
			SyncEligibleReason: SyncReasonCatalogDigestError,
			Detail:             fmt.Sprintf("failed to compute catalog digest: %v", err),
		}
	}

	// Check if manifest producer matches the running binary catalog
	producerMatches := (manifest.Producer.CatalogDigest == currentCatalogDigest) &&
		(manifest.Producer.Version == catalog.Version) &&
		(catalog.VCSRevision == "" || manifest.Producer.VCSRevision == catalog.VCSRevision)

	if producerMatches {
		// Validate that manifest.Resources exactly matches current catalog descriptors
		if len(manifest.Resources) != len(catalog.Resources) {
			return ClassificationResult{
				BundleIdentity:     BundleIdentityMixed,
				ExtentIntegrity:    ExtentIntegrityUserModified,
				RecoveryState:      RecoveryStateNone,
				SyncEligible:       false,
				SyncEligibleReason: SyncReasonMixedResourceTransactions,
				Detail:             "manifest resources count does not match current catalog",
			}
		}

		manifestMap := make(map[string]ResourceEntry, len(manifest.Resources))
		for _, r := range manifest.Resources {
			manifestMap[r.ResourceID] = r
		}

		for _, desc := range catalog.Resources {
			res, ok := manifestMap[desc.ResourceID]
			if !ok {
				return ClassificationResult{
					BundleIdentity:     BundleIdentityMixed,
					ExtentIntegrity:    ExtentIntegrityUserModified,
					RecoveryState:      RecoveryStateNone,
					SyncEligible:       false,
					SyncEligibleReason: SyncReasonMixedResourceTransactions,
					Detail:             fmt.Sprintf("manifest missing resource descriptor %s", desc.ResourceID),
				}
			}
			desired, err := desc.RenderDesired()
			if err != nil {
				return ClassificationResult{
					BundleIdentity:     BundleIdentityUnknown,
					ExtentIntegrity:    ExtentIntegrityMatch,
					RecoveryState:      RecoveryStateNone,
					SyncEligible:       false,
					SyncEligibleReason: SyncReasonCatalogDigestError,
					Detail:             fmt.Sprintf("failed to render desired extent: %v", err),
				}
			}
			expectedTarget := TargetIdentityToken(desc.CanonicalPath)
			if res.TargetIdentity != expectedTarget || res.CanonicalPath != desc.CanonicalPath || res.Ownership.Kind != desc.Ownership.Kind || res.DesiredSHA256 != desired.SHA256 || res.ObservedSHA256 != desired.SHA256 || (desired.Mode != 0 && res.Mode != desired.Mode) {
				return ClassificationResult{
					BundleIdentity:     BundleIdentityUserModified,
					ExtentIntegrity:    ExtentIntegrityUserModified,
					RecoveryState:      RecoveryStateNone,
					SyncEligible:       false,
					SyncEligibleReason: SyncReasonContentMismatch,
					Detail:             fmt.Sprintf("manifest resource %s descriptor mismatch with running catalog", desc.ResourceID),
				}
			}
		}

		return ClassificationResult{
			BundleIdentity:     BundleIdentityAligned,
			ExtentIntegrity:    ExtentIntegrityMatch,
			RecoveryState:      RecoveryStateNone,
			SyncEligible:       true,
			SyncEligibleReason: SyncReasonAlignedWithCurrentBinary,
			Detail:             fmt.Sprintf("installed managed assets match running binary (%s)", catalog.Version),
		}
	}

	// Trusted older producer with exact committed extents
	return ClassificationResult{
		BundleIdentity:     BundleIdentityStale,
		ExtentIntegrity:    ExtentIntegrityMatch,
		RecoveryState:      RecoveryStateNone,
		SyncEligible:       true,
		SyncEligibleReason: SyncReasonExactCommittedOlderBundle,
		Detail: fmt.Sprintf("installed assets were configured by gentle-ai %s, but running binary is %s — run 'gentle-ai sync' to update installed assets",
			manifest.Producer.Version, catalog.Version),
	}
}

func checkActiveTransactions(homeDir string, manifest ManagedBundleManifest) (RecoveryState, string) {
	txDir := filepath.Join(homeDir, ManagedDirName, ManagedSubDir, JournalDirName)
	entries, err := os.ReadDir(txDir)
	if err != nil {
		if os.IsNotExist(err) {
			return RecoveryStateNone, ""
		}
		return RecoveryStateBlockedConflict, fmt.Sprintf("cannot read transactions directory: %v", err)
	}
	if len(entries) == 0 {
		return RecoveryStateNone, ""
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		journalPath := filepath.Join(txDir, entry.Name(), JournalFileName)
		data, err := os.ReadFile(journalPath)
		if err != nil {
			return RecoveryStateBlockedConflict, fmt.Sprintf("cannot read transaction journal %s: %v", entry.Name(), err)
		}
		var journal MigrationJournal
		if err := json.Unmarshal(data, &journal); err != nil {
			return RecoveryStateBlockedConflict, fmt.Sprintf("malformed transaction journal %s: %v", entry.Name(), err)
		}

		if journal.Schema != JournalSchemaV1 {
			return RecoveryStateBlockedConflict, fmt.Sprintf("unsupported transaction journal schema %q in %s", journal.Schema, entry.Name())
		}

		if journal.LastPhase == PhaseCompleted || journal.LastPhase == PhaseRolledBack {
			continue
		}

		// If the manifest already names this transaction, it was committed
		if manifest.TransactionID == journal.TransactionID && manifest.Generation == journal.ProposedGeneration {
			continue
		}

		// Non-terminal journals whose ExpectedGeneration is lower than current manifest generation are superseded
		if manifest.Generation > 0 && journal.ExpectedGeneration < manifest.Generation {
			continue
		}

		// If the journal declares no resources, it cannot provide recovery evidence
		if len(journal.Resources) == 0 {
			return RecoveryStateBlockedConflict, fmt.Sprintf("transaction %s contains no resource entries", journal.TransactionID)
		}

		// Evaluate on-disk files against journal Before / Desired states
		hasForeign := false
		allDesired := true
		allBefore := true

		for _, res := range journal.Resources {
			targetPath, pathErr := resolveSafeCanonicalPath(homeDir, res.CanonicalPath)
			if pathErr != nil {
				hasForeign = true
				break
			}
			fi, err := os.Lstat(targetPath)
			if err != nil {
				if os.IsNotExist(err) && res.BeforeSHA256 == "" {
					// file didn't exist before
					allDesired = false
					continue
				}
				hasForeign = true
				break
			}
			if !fi.Mode().IsRegular() {
				hasForeign = true
				break
			}
			fileMode := uint32(fi.Mode().Perm())

			content, err := os.ReadFile(targetPath)
			if err != nil {
				hasForeign = true
				break
			}
			sum := sha256.Sum256(content)
			fileSHA := "sha256:" + hex.EncodeToString(sum[:])

			matchesDesired := (fileSHA == res.DesiredSHA256) && (res.DesiredMode == 0 || fileMode == res.DesiredMode)
			matchesBefore := (fileSHA == res.BeforeSHA256) && (res.BeforeMode == 0 || fileMode == res.BeforeMode)

			if matchesDesired && !matchesBefore {
				allBefore = false
			} else if matchesBefore && !matchesDesired {
				allDesired = false
			} else if matchesDesired && matchesBefore {
				// identical before and desired
			} else {
				hasForeign = true
				break
			}
		}

		if hasForeign {
			return RecoveryStateBlockedConflict, fmt.Sprintf("transaction %s interrupted with conflicting modifications", journal.TransactionID)
		}
		if allDesired {
			return RecoveryStateResumableDesired, fmt.Sprintf("transaction %s interrupted with desired state on disk — resumable commit", journal.TransactionID)
		}
		if allBefore {
			return RecoveryStateResumableBefore, fmt.Sprintf("transaction %s interrupted with before state preserved — recoverable", journal.TransactionID)
		}
		return RecoveryStateBlockedConflict, fmt.Sprintf("transaction %s in partial state", journal.TransactionID)
	}

	return RecoveryStateNone, ""
}
