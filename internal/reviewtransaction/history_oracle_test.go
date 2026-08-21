package reviewtransaction

import (
	"sync"
	"sync/atomic"
	"testing"
)

func TestOracleScheduleManifestNonZero(t *testing.T) {
	manifest := CanonicalScheduleManifest()
	if len(manifest) == 0 {
		t.Fatal("CanonicalScheduleManifest() returned empty list, want non-zero schedule count")
	}
	for _, entry := range manifest {
		if entry.Name == "" {
			t.Fatal("manifest entry missing name")
		}
		if entry.Description == "" {
			t.Fatalf("manifest entry %q missing description", entry.Name)
		}
		if entry.Actors <= 0 || entry.Operations <= 0 {
			t.Fatalf("manifest entry %q has invalid bounds (actors=%d, ops=%d)", entry.Name, entry.Actors, entry.Operations)
		}
	}
}

func TestOracleScenario1ConcurrentStart(t *testing.T) {
	checker := LinearizabilityChecker{}
	var clock atomic.Int64
	getTime := func() int64 { return clock.Add(1) }

	var historyMu sync.Mutex
	var history []HistoryEvent

	var authorityMu sync.Mutex
	authorityState := OracleAuthorityAbsent

	startBarrier := make(chan struct{})
	var wg sync.WaitGroup
	actors := []string{"actor-1", "actor-2", "actor-3"}
	wg.Add(len(actors))

	for _, actor := range actors {
		go func(act string) {
			defer wg.Done()
			<-startBarrier
			t0 := getTime()

			authorityMu.Lock()
			var resultCode string
			if authorityState == OracleAuthorityAbsent {
				authorityState = OracleAuthorityReviewing
				resultCode = "created"
			} else {
				resultCode = "resumed"
			}
			authorityMu.Unlock()

			t1 := getTime()

			event := HistoryEvent{
				InvocationID:     act + "-inv",
				ResponseID:       act + "-resp",
				StartTime:        t0,
				EndTime:          t1,
				Actor:            act,
				Operation:        HistoryOpStart,
				LineageID:        "lin-1",
				IdempotencyKey:   "key-1",
				ObservedRevision: "rev-1",
				ResultCode:       resultCode,
				PreAuthority:     OracleAuthorityAbsent,
				PostAuthority:    OracleAuthorityReviewing,
			}

			historyMu.Lock()
			history = append(history, event)
			historyMu.Unlock()
		}(actor)
	}

	close(startBarrier)
	wg.Wait()

	SortHistoryEvents(history)

	linearized, err := checker.CheckLinearizability(history)
	if err != nil {
		t.Fatalf("CheckLinearizability() failed: %v", err)
	}
	if len(linearized) != len(history) {
		t.Fatalf("linearized events count = %d, want %d", len(linearized), len(history))
	}

	if err := checker.CheckLiveness(history, DefaultLivenessBounds()); err != nil {
		t.Fatalf("CheckLiveness() failed: %v", err)
	}

	createdCount := 0
	resumedCount := 0
	for _, e := range history {
		if e.ResultCode == "created" {
			createdCount++
		} else if e.ResultCode == "resumed" {
			resumedCount++
		}
	}
	if createdCount != 1 {
		t.Fatalf("expected exactly 1 creator/budget owner, got %d", createdCount)
	}
	if resumedCount != 2 {
		t.Fatalf("expected 2 resumed callers, got %d", resumedCount)
	}
}

func TestOracleScenario2ConcurrentFinalize(t *testing.T) {
	checker := LinearizabilityChecker{}
	var clock atomic.Int64
	getTime := func() int64 { return clock.Add(1) }

	var historyMu sync.Mutex
	var history []HistoryEvent

	// Seed start event
	t0 := getTime()
	t1 := getTime()
	history = append(history, HistoryEvent{
		InvocationID:     "start-inv",
		ResponseID:       "start-resp",
		StartTime:        t0,
		EndTime:          t1,
		Actor:            "creator",
		Operation:        HistoryOpStart,
		LineageID:        "lin-2",
		IdempotencyKey:   "key-2",
		ObservedRevision: "rev-1",
		ResultCode:       "created",
		PreAuthority:     OracleAuthorityAbsent,
		PostAuthority:    OracleAuthorityReviewing,
	})

	var authorityMu sync.Mutex
	authorityState := OracleAuthorityReviewing
	authorityRev := "rev-1"

	finalizeBarrier := make(chan struct{})
	var wg sync.WaitGroup
	actors := []string{"fin-1", "fin-2", "fin-3"}
	wg.Add(len(actors))

	for _, actor := range actors {
		go func(act string) {
			defer wg.Done()
			<-finalizeBarrier
			startT := getTime()

			authorityMu.Lock()
			var resultCode string
			var obsRev string
			var receiptIssued bool
			var preAuth OracleAuthorityState
			if authorityState == OracleAuthorityReviewing {
				authorityState = OracleAuthorityApproved
				authorityRev = "rev-2"
				resultCode = "approved"
				obsRev = "rev-2"
				receiptIssued = true
				preAuth = OracleAuthorityReviewing
			} else {
				resultCode = "idempotent_terminal"
				obsRev = authorityRev
				receiptIssued = false
				preAuth = OracleAuthorityApproved
			}
			authorityMu.Unlock()

			endT := getTime()

			event := HistoryEvent{
				InvocationID:     act + "-inv",
				ResponseID:       act + "-resp",
				StartTime:        startT,
				EndTime:          endT,
				Actor:            act,
				Operation:        HistoryOpFinalize,
				LineageID:        "lin-2",
				ExpectedRevision: "rev-1",
				ObservedRevision: obsRev,
				ResultCode:       resultCode,
				ReceiptIssued:    receiptIssued,
				PreAuthority:     preAuth,
				PostAuthority:    OracleAuthorityApproved,
			}

			historyMu.Lock()
			history = append(history, event)
			historyMu.Unlock()
		}(actor)
	}

	close(finalizeBarrier)
	wg.Wait()

	SortHistoryEvents(history)

	linearized, err := checker.CheckLinearizability(history)
	if err != nil {
		t.Fatalf("CheckLinearizability() failed: %v", err)
	}
	if len(linearized) != len(history) {
		t.Fatalf("linearized events count = %d, want %d", len(linearized), len(history))
	}

	if err := checker.CheckLiveness(history, DefaultLivenessBounds()); err != nil {
		t.Fatalf("CheckLiveness() failed: %v", err)
	}

	approvedCount := 0
	for _, e := range history {
		if e.Operation == HistoryOpFinalize && e.ResultCode == "approved" {
			approvedCount++
		}
	}
	if approvedCount != 1 {
		t.Fatalf("expected exactly 1 commit to approved, got %d", approvedCount)
	}
}

func TestOracleScenario3ReadOnlyGatesDuringFinalize(t *testing.T) {
	checker := LinearizabilityChecker{}
	var clock atomic.Int64
	getTime := func() int64 { return clock.Add(1) }

	var history []HistoryEvent

	// 1. START
	t0 := getTime()
	t1 := getTime()
	history = append(history, HistoryEvent{
		InvocationID:     "start-inv",
		ResponseID:       "start-resp",
		StartTime:        t0,
		EndTime:          t1,
		Actor:            "writer",
		Operation:        HistoryOpStart,
		LineageID:        "lin-3",
		IdempotencyKey:   "key-3",
		ObservedRevision: "rev-1",
		ResultCode:       "created",
		PreAuthority:     OracleAuthorityAbsent,
		PostAuthority:    OracleAuthorityReviewing,
	})

	// 2. Gate while reviewing -> blocked
	t2 := getTime()
	t3 := getTime()
	history = append(history, HistoryEvent{
		InvocationID:  "gate1-inv",
		ResponseID:    "gate1-resp",
		StartTime:     t2,
		EndTime:       t3,
		Actor:         "gate-reader-1",
		Operation:     HistoryOpValidate,
		LineageID:     "lin-3",
		ResultCode:    "blocked",
		PreAuthority:  OracleAuthorityReviewing,
		PostAuthority: OracleAuthorityReviewing,
	})

	// 3. FINALIZE -> approved
	t4 := getTime()
	t5 := getTime()
	history = append(history, HistoryEvent{
		InvocationID:     "fin-inv",
		ResponseID:       "fin-resp",
		StartTime:        t4,
		EndTime:          t5,
		Actor:            "writer",
		Operation:        HistoryOpFinalize,
		LineageID:        "lin-3",
		ExpectedRevision: "rev-1",
		ObservedRevision: "rev-2",
		ResultCode:       "approved",
		ReceiptIssued:    true,
		PreAuthority:     OracleAuthorityReviewing,
		PostAuthority:    OracleAuthorityApproved,
	})

	// 4. Gate after approved -> allow
	t6 := getTime()
	t7 := getTime()
	history = append(history, HistoryEvent{
		InvocationID:  "gate2-inv",
		ResponseID:    "gate2-resp",
		StartTime:     t6,
		EndTime:       t7,
		Actor:         "gate-reader-2",
		Operation:     HistoryOpValidate,
		LineageID:     "lin-3",
		ResultCode:    "allow",
		PreAuthority:  OracleAuthorityApproved,
		PostAuthority: OracleAuthorityApproved,
	})

	linearized, err := checker.CheckLinearizability(history)
	if err != nil {
		t.Fatalf("CheckLinearizability() failed: %v", err)
	}
	if len(linearized) != 4 {
		t.Fatalf("linearized count = %d, want 4", len(linearized))
	}
	if err := checker.CheckLiveness(history, DefaultLivenessBounds()); err != nil {
		t.Fatalf("CheckLiveness() failed: %v", err)
	}
}

func TestOracleScenario4CrashBeforeEffectPublication(t *testing.T) {
	checker := LinearizabilityChecker{}
	var clock atomic.Int64
	getTime := func() int64 { return clock.Add(1) }

	var history []HistoryEvent

	// 1. START
	history = append(history, HistoryEvent{
		InvocationID:     "start-inv",
		ResponseID:       "start-resp",
		StartTime:        getTime(),
		EndTime:          getTime(),
		Actor:            "writer",
		Operation:        HistoryOpStart,
		LineageID:        "lin-4",
		IdempotencyKey:   "key-4",
		ObservedRevision: "rev-1",
		ResultCode:       "created",
		PreAuthority:     OracleAuthorityAbsent,
		PostAuthority:    OracleAuthorityReviewing,
	})

	// 2. FINALIZE (committed to authority, pending effect)
	history = append(history, HistoryEvent{
		InvocationID:     "fin-inv",
		ResponseID:       "fin-resp",
		StartTime:        getTime(),
		EndTime:          getTime(),
		Actor:            "writer",
		Operation:        HistoryOpFinalize,
		LineageID:        "lin-4",
		ExpectedRevision: "rev-1",
		ObservedRevision: "rev-2",
		ResultCode:       "approved",
		ReceiptIssued:    true,
		PreAuthority:     OracleAuthorityReviewing,
		PostAuthority:    OracleAuthorityApproved,
		PreEffect:        OracleEffectNone,
		PostEffect:       OracleEffectPending,
	})

	// 3. Post-crash STATUS check must route to RECONCILE, never START
	history = append(history, HistoryEvent{
		InvocationID:  "status-inv",
		ResponseID:    "status-resp",
		StartTime:     getTime(),
		EndTime:       getTime(),
		Actor:         "recovery-reader",
		Operation:     HistoryOpStatus,
		LineageID:     "lin-4",
		ResultCode:    "reconcile",
		PreAuthority:  OracleAuthorityApproved,
		PostAuthority: OracleAuthorityApproved,
		PreEffect:     OracleEffectPending,
		PostEffect:    OracleEffectPending,
	})

	// 4. RECONCILE executes and applies effect
	history = append(history, HistoryEvent{
		InvocationID:  "rec-inv",
		ResponseID:    "rec-resp",
		StartTime:     getTime(),
		EndTime:       getTime(),
		Actor:         "reconciler",
		Operation:     HistoryOpReconcile,
		LineageID:     "lin-4",
		ResultCode:    "applied",
		PreAuthority:  OracleAuthorityApproved,
		PostAuthority: OracleAuthorityApproved,
		PreEffect:     OracleEffectPending,
		PostEffect:    OracleEffectApplied,
	})

	// 5. Final STATUS is complete
	history = append(history, HistoryEvent{
		InvocationID:  "final-status-inv",
		ResponseID:    "final-status-resp",
		StartTime:     getTime(),
		EndTime:       getTime(),
		Actor:         "recovery-reader",
		Operation:     HistoryOpStatus,
		LineageID:     "lin-4",
		ResultCode:    "complete",
		PreAuthority:  OracleAuthorityApproved,
		PostAuthority: OracleAuthorityApproved,
		PreEffect:     OracleEffectApplied,
		PostEffect:    OracleEffectApplied,
	})

	linearized, err := checker.CheckLinearizability(history)
	if err != nil {
		t.Fatalf("CheckLinearizability() failed: %v", err)
	}
	if len(linearized) != 5 {
		t.Fatalf("linearized count = %d, want 5", len(linearized))
	}
	if err := checker.CheckLiveness(history, DefaultLivenessBounds()); err != nil {
		t.Fatalf("CheckLiveness() failed: %v", err)
	}
}

func TestOracleScenario5ConcurrentReconciliation(t *testing.T) {
	checker := LinearizabilityChecker{}
	var clock atomic.Int64
	getTime := func() int64 { return clock.Add(1) }

	var historyMu sync.Mutex
	var history []HistoryEvent

	// Setup: approved authority with pending effect
	history = append(history,
		HistoryEvent{
			InvocationID:     "start-inv",
			ResponseID:       "start-resp",
			StartTime:        getTime(),
			EndTime:          getTime(),
			Actor:            "creator",
			Operation:        HistoryOpStart,
			LineageID:        "lin-5",
			IdempotencyKey:   "key-5",
			ObservedRevision: "rev-1",
			ResultCode:       "created",
		},
		HistoryEvent{
			InvocationID:     "fin-inv",
			ResponseID:       "fin-resp",
			StartTime:        getTime(),
			EndTime:          getTime(),
			Actor:            "creator",
			Operation:        HistoryOpFinalize,
			LineageID:        "lin-5",
			ExpectedRevision: "rev-1",
			ObservedRevision: "rev-2",
			ResultCode:       "approved",
			ReceiptIssued:    true,
		},
	)

	var effectMu sync.Mutex
	effectState := OracleEffectPending

	barrier := make(chan struct{})
	var wg sync.WaitGroup
	actors := []string{"rec-1", "rec-2", "rec-3"}
	wg.Add(len(actors))

	for _, actor := range actors {
		go func(act string) {
			defer wg.Done()
			<-barrier
			startT := getTime()

			effectMu.Lock()
			var resultCode string
			if effectState == OracleEffectPending {
				effectState = OracleEffectApplied
				resultCode = "applied"
			} else {
				resultCode = "idempotent"
			}
			effectMu.Unlock()

			endT := getTime()

			event := HistoryEvent{
				InvocationID: "rec-" + act + "-inv",
				ResponseID:   "rec-" + act + "-resp",
				StartTime:    startT,
				EndTime:      endT,
				Actor:        act,
				Operation:    HistoryOpReconcile,
				LineageID:    "lin-5",
				ResultCode:   resultCode,
			}

			historyMu.Lock()
			history = append(history, event)
			historyMu.Unlock()
		}(actor)
	}

	close(barrier)
	wg.Wait()

	SortHistoryEvents(history)

	linearized, err := checker.CheckLinearizability(history)
	if err != nil {
		t.Fatalf("CheckLinearizability() failed: %v", err)
	}
	if len(linearized) != len(history) {
		t.Fatalf("linearized events count = %d, want %d", len(linearized), len(history))
	}

	appliedIdx := -1
	for i, e := range linearized {
		if e.Operation == HistoryOpReconcile && e.ResultCode == "applied" {
			appliedIdx = i
			break
		}
	}
	if appliedIdx == -1 {
		t.Fatal("applied reconcile event not found in linearized history")
	}
	for i, e := range linearized {
		if e.Operation == HistoryOpReconcile && e.ResultCode == "idempotent" && i < appliedIdx {
			t.Fatalf("idempotent reconcile at index %d appeared before applied reconcile at index %d", i, appliedIdx)
		}
	}

	if err := checker.CheckLiveness(history, DefaultLivenessBounds()); err != nil {
		t.Fatalf("CheckLiveness() failed: %v", err)
	}

	appliedCount := 0
	for _, e := range history {
		if e.Operation == HistoryOpReconcile && e.ResultCode == "applied" {
			appliedCount++
		}
	}
	if appliedCount != 1 {
		t.Fatalf("expected exactly 1 applied effect transition, got %d", appliedCount)
	}
}

func TestOracleLinearizabilityRejectsNonSerializableHistory(t *testing.T) {
	checker := LinearizabilityChecker{}

	// Illegal history: finalize succeeds before start completed
	history := []HistoryEvent{
		{
			InvocationID: "fin-inv",
			ResponseID:   "fin-resp",
			StartTime:    1,
			EndTime:      2,
			Actor:        "actor-1",
			Operation:    HistoryOpFinalize,
			LineageID:    "lin-fail",
			ResultCode:   "approved",
		},
		{
			InvocationID: "start-inv",
			ResponseID:   "start-resp",
			StartTime:    3,
			EndTime:      4,
			Actor:        "actor-2",
			Operation:    HistoryOpStart,
			LineageID:    "lin-fail",
			ResultCode:   "created",
		},
	}

	if _, err := checker.CheckLinearizability(history); err == nil {
		t.Fatal("CheckLinearizability() succeeded on illegal history, want error")
	}
}

func TestOracleLivenessRejectsBudgetExceeded(t *testing.T) {
	checker := LinearizabilityChecker{}

	// Excessive CAS attempts exceeding bounds
	history := []HistoryEvent{
		{Actor: "actor-1", Operation: HistoryOpFinalize, ResultCode: "stale_cas"},
		{Actor: "actor-1", Operation: HistoryOpFinalize, ResultCode: "stale_cas"},
		{Actor: "actor-1", Operation: HistoryOpFinalize, ResultCode: "stale_cas"},
	}

	bounds := LivenessBounds{MaxTransitionsToTerminal: 4, MaxCASAttemptsPerActor: 2}
	if err := checker.CheckLiveness(history, bounds); err == nil {
		t.Fatal("CheckLiveness() succeeded on budget exceeded history, want error")
	}
}
