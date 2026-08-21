package managedbundle

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

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
		targetPath := filepath.Join(homeDir, filepath.FromSlash(res.CanonicalPath))
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

		if fi.Mode()&os.ModeSymlink != 0 || fi.IsDir() {
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
	if manifest.Producer.CatalogDigest == currentCatalogDigest {
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
			targetPath := filepath.Join(homeDir, filepath.FromSlash(res.CanonicalPath))
			content, err := os.ReadFile(targetPath)
			if err != nil {
				if os.IsNotExist(err) && res.BeforeSHA256 == "" {
					// file didn't exist before
					allDesired = false
					continue
				}
				hasForeign = true
				break
			}
			sum := sha256.Sum256(content)
			fileSHA := "sha256:" + hex.EncodeToString(sum[:])

			if fileSHA == res.DesiredSHA256 {
				allBefore = false
			} else if fileSHA == res.BeforeSHA256 {
				allDesired = false
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
