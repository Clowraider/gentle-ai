package reviewtransaction

import (
	"context"
	"errors"
	"sync"
	"testing"
)

type recordingHistoryScheduler struct {
	mu     sync.Mutex
	points []HistoryPoint
	fail   HistoryPoint
}

var errHistoryBoundary = errors.New("history boundary reached")

func (scheduler *recordingHistoryScheduler) Reach(_ context.Context, _ string, point HistoryPoint, _ HistoryBoundary) error {
	scheduler.mu.Lock()
	defer scheduler.mu.Unlock()
	scheduler.points = append(scheduler.points, point)
	if point == scheduler.fail {
		return errHistoryBoundary
	}
	return nil
}

func TestHistorySchedulerReachesRealAtomicStartBoundaries(t *testing.T) {
	repo := initSnapshotRepo(t)
	request := compactAtomicStartFixture(t, repo, "scheduler-start")
	store, err := CompactAuthoritativeStore(context.Background(), repo, request.State.LineageID)
	if err != nil {
		t.Fatal(err)
	}
	scheduler := &recordingHistoryScheduler{}
	ctx := WithHistoryScheduler(context.Background(), scheduler)
	if _, err := store.CreateOrReplayAtomicStart(ctx, request); err != nil {
		t.Fatal(err)
	}
	assertHistoryPoints(t, scheduler.points, HistoryBeforeLock, HistoryAfterLock, HistoryBeforeCAS, HistoryAfterCommit, HistoryBeforeResponse)
}

func TestHistorySchedulerReachesRealFinalizeTransitionBoundaries(t *testing.T) {
	repo := initSnapshotRepo(t)
	state := newCompactTestState(t, repo, "scheduler-finalize")
	store := storeCompactStartAuthority(t, repo, state)
	record, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	next := record.State
	if err := next.CompleteReview(CompactReviewInput{}); err != nil {
		t.Fatal(err)
	}
	scheduler := &recordingHistoryScheduler{}
	ctx := WithHistoryScheduler(context.Background(), scheduler)
	if _, err := store.ReplaceContext(ctx, record.Revision, "review/complete-review", next); err != nil {
		t.Fatal(err)
	}
	assertHistoryPoints(t, scheduler.points, HistoryBeforeLock, HistoryAfterLock, HistoryAfterRead, HistoryBeforeCAS, HistoryAfterCommit, HistoryBeforeResponse)
}

func TestHistorySchedulerReachesReadOnlyAndReconcileProductionPaths(t *testing.T) {
	checks := []struct {
		name string
		run  func(context.Context) error
	}{
		{name: "status", run: func(ctx context.Context) error {
			_, _, err := AssessTargetStatusWithSnapshot(ctx, t.TempDir(), TargetStatusRequest{})
			return err
		}},
		{name: "validate", run: func(ctx context.Context) error {
			_, err := AssessCompactGateTarget(ctx, t.TempDir(), CompactState{}, NativeGateRequestInput{})
			return err
		}},
		{name: "reconcile", run: func(ctx context.Context) error {
			_, err := ReconcileCompactRepositoryContext(ctx, CompactStore{}, CompactRecord{})
			return err
		}},
	}
	for _, check := range checks {
		t.Run(check.name, func(t *testing.T) {
			scheduler := &recordingHistoryScheduler{fail: HistoryAfterRead}
			if err := check.run(WithHistoryScheduler(context.Background(), scheduler)); !errors.Is(err, errHistoryBoundary) {
				t.Fatalf("production path error = %v, want scheduler boundary error", err)
			}
			assertHistoryPoints(t, scheduler.points, HistoryAfterRead)
		})
	}
}

func assertHistoryPoints(t *testing.T, got []HistoryPoint, want ...HistoryPoint) {
	t.Helper()
	for _, expected := range want {
		found := false
		for _, point := range got {
			found = found || point == expected
		}
		if !found {
			t.Fatalf("points = %v, missing %q", got, expected)
		}
	}
}
