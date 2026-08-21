// File history_oracle.go — bounded sequential reference model, history recording,
// and linearizability oracle for review authority transitions (Issue #1874 bounded v1).

package reviewtransaction

import (
	"errors"
	"fmt"
	"sort"
)

// MaxHistoryEventsBound is the upper limit of events allowed in a single linearizability check.
const MaxHistoryEventsBound = 64

// HistoryOperation identifies an operation in the bounded v1 authority oracle.
type HistoryOperation string

const (
	HistoryOpStart     HistoryOperation = "START"
	HistoryOpFinalize  HistoryOperation = "FINALIZE"
	HistoryOpStatus    HistoryOperation = "STATUS"
	HistoryOpValidate  HistoryOperation = "VALIDATE"
	HistoryOpReconcile HistoryOperation = "RECONCILE"
)

// OracleAuthorityState represents the authority state in the bounded reference model.
type OracleAuthorityState string

const (
	OracleAuthorityAbsent    OracleAuthorityState = "absent"
	OracleAuthorityReviewing OracleAuthorityState = "reviewing"
	OracleAuthorityApproved  OracleAuthorityState = "approved"
)

// OracleEffectState represents the effect marker state in the bounded reference model.
type OracleEffectState string

const (
	OracleEffectNone    OracleEffectState = "none"
	OracleEffectPending OracleEffectState = "pending"
	OracleEffectApplied OracleEffectState = "applied"
	OracleEffectBlocked OracleEffectState = "blocked"
)

// HistoryEvent records the invocation, execution bounds, and observed state of one operation.
type HistoryEvent struct {
	InvocationID     string               `json:"invocation_id"`
	ResponseID       string               `json:"response_id"`
	StartTime        int64                `json:"start_time"`
	EndTime          int64                `json:"end_time"`
	Actor            string               `json:"actor"`
	Operation        HistoryOperation     `json:"operation"`
	LineageID        string               `json:"lineage_id"`
	IdempotencyKey   string               `json:"idempotency_key"`
	ExpectedRevision string               `json:"expected_revision"`
	ResultCode       string               `json:"result_code"`
	MutationOutcome  string               `json:"mutation_outcome"`
	ObservedRevision string               `json:"observed_revision"`
	ReceiptIssued    bool                 `json:"receipt_issued"`
	PreAuthority     OracleAuthorityState `json:"pre_authority"`
	PostAuthority    OracleAuthorityState `json:"post_authority"`
	PreEffect        OracleEffectState    `json:"pre_effect"`
	PostEffect       OracleEffectState    `json:"post_effect"`
}

// OracleModelState captures the single-lineage sequential reference model state.
type OracleModelState struct {
	Authority       OracleAuthorityState
	AuthorityRev    string
	BudgetOwner     string
	IdempotencyKey  string
	ReceiptIssued   bool
	Effect          OracleEffectState
	CommittedEvents int
}

// InitialOracleModelState returns an initial empty model state.
func InitialOracleModelState() OracleModelState {
	return OracleModelState{
		Authority: OracleAuthorityAbsent,
		Effect:    OracleEffectNone,
	}
}

// Step evaluates a single operation against the sequential model.
// Returns the next state, whether the step was legal, and any violation reason.
func (s OracleModelState) Step(e HistoryEvent) (OracleModelState, bool, string) {
	next := s
	switch e.Operation {
	case HistoryOpStart:
		switch s.Authority {
		case OracleAuthorityAbsent:
			if e.ResultCode != "success" && e.ResultCode != "created" {
				return s, false, fmt.Sprintf("start on absent authority returned unexpected result %q", e.ResultCode)
			}
			next.Authority = OracleAuthorityReviewing
			next.AuthorityRev = e.ObservedRevision
			next.BudgetOwner = e.Actor
			next.IdempotencyKey = e.IdempotencyKey
			next.CommittedEvents++
			return next, true, ""
		case OracleAuthorityReviewing:
			if s.IdempotencyKey == e.IdempotencyKey {
				if e.ResultCode != "success" && e.ResultCode != "resumed" {
					return s, false, fmt.Sprintf("idempotent start on reviewing authority returned unexpected result %q", e.ResultCode)
				}
				return s, true, ""
			}
			if e.ResultCode != "refused_contention" && e.ResultCode != "stale_cas" {
				return s, false, fmt.Sprintf("conflicting start on reviewing authority returned %q without contention refusal", e.ResultCode)
			}
			return s, true, ""
		case OracleAuthorityApproved:
			if e.ResultCode != "success" && e.ResultCode != "resumed" && e.ResultCode != "approved" {
				return s, false, fmt.Sprintf("start on approved authority returned unexpected result %q", e.ResultCode)
			}
			return s, true, ""
		default:
			return s, false, fmt.Sprintf("unhandled authority state %q for start operation", s.Authority)
		}

	case HistoryOpFinalize:
		switch s.Authority {
		case OracleAuthorityAbsent:
			if e.ResultCode != "error" && e.ResultCode != "missing_authority" {
				return s, false, fmt.Sprintf("finalize on absent authority returned unexpected result %q", e.ResultCode)
			}
			return s, true, ""
		case OracleAuthorityReviewing:
			if e.ExpectedRevision != "" && s.AuthorityRev != "" && e.ExpectedRevision != s.AuthorityRev {
				if e.ResultCode != "stale_cas" && e.ResultCode != "refused_contention" {
					return s, false, fmt.Sprintf("finalize with stale revision %q (current %q) succeeded without CAS failure", e.ExpectedRevision, s.AuthorityRev)
				}
				return s, true, ""
			}
			if e.ResultCode != "success" && e.ResultCode != "approved" {
				return s, false, fmt.Sprintf("finalize on reviewing authority returned unexpected result %q", e.ResultCode)
			}
			next.Authority = OracleAuthorityApproved
			next.AuthorityRev = e.ObservedRevision
			next.ReceiptIssued = true
			next.Effect = OracleEffectPending
			next.CommittedEvents++
			return next, true, ""
		case OracleAuthorityApproved:
			if e.ResultCode != "success" && e.ResultCode != "approved" && e.ResultCode != "idempotent_terminal" {
				return s, false, fmt.Sprintf("finalize on already approved authority returned unexpected result %q", e.ResultCode)
			}
			return s, true, ""
		default:
			return s, false, fmt.Sprintf("unhandled authority state %q for finalize operation", s.Authority)
		}

	case HistoryOpStatus:
		switch s.Authority {
		case OracleAuthorityAbsent:
			if e.ResultCode != "absent" && e.ResultCode != "sdd-new" && e.ResultCode != "start" {
				return s, false, fmt.Sprintf("status on absent authority returned %q, expected start routing", e.ResultCode)
			}
			return s, true, ""
		case OracleAuthorityReviewing:
			if e.ResultCode != "reviewing" && e.ResultCode != "validate" && e.ResultCode != "finalize" {
				return s, false, fmt.Sprintf("status on reviewing authority returned %q, expected validate/finalize", e.ResultCode)
			}
			return s, true, ""
		case OracleAuthorityApproved:
			if s.Effect == OracleEffectPending {
				if e.ResultCode != "reconcile" && e.ResultCode != "reconcile_finalize" {
					return s, false, fmt.Sprintf("status on approved authority with pending effect returned %q, expected reconcile", e.ResultCode)
				}
				return s, true, ""
			}
			if e.ResultCode != "complete" && e.ResultCode != "allow" && e.ResultCode != "approved" {
				return s, false, fmt.Sprintf("status on approved authority with applied effect returned %q", e.ResultCode)
			}
			return s, true, ""
		default:
			return s, false, fmt.Sprintf("unhandled authority state %q for status operation", s.Authority)
		}

	case HistoryOpValidate:
		switch s.Authority {
		case OracleAuthorityAbsent:
			if e.ResultCode != "blocked" && e.ResultCode != "missing_authority" {
				return s, false, fmt.Sprintf("validate on absent authority returned %q, expected blocked", e.ResultCode)
			}
			return s, true, ""
		case OracleAuthorityReviewing:
			if e.ResultCode != "blocked" && e.ResultCode != "reviewing" && e.ResultCode != "retryable" {
				return s, false, fmt.Sprintf("validate on reviewing authority returned %q, expected blocked/retryable", e.ResultCode)
			}
			return s, true, ""
		case OracleAuthorityApproved:
			if e.ResultCode != "allow" && e.ResultCode != "success" {
				return s, false, fmt.Sprintf("validate on approved authority returned %q, expected allow", e.ResultCode)
			}
			return s, true, ""
		default:
			return s, false, fmt.Sprintf("unhandled authority state %q for validate operation", s.Authority)
		}

	case HistoryOpReconcile:
		switch s.Effect {
		case OracleEffectNone, OracleEffectPending:
			if s.Authority != OracleAuthorityApproved {
				if e.ResultCode != "blocked" && e.ResultCode != "error" {
					return s, false, fmt.Sprintf("reconcile on unapproved authority returned %q", e.ResultCode)
				}
				return s, true, ""
			}
			if e.ResultCode != "applied" && e.ResultCode != "success" {
				return s, false, fmt.Sprintf("reconcile on pending effect returned unexpected result %q", e.ResultCode)
			}
			next.Effect = OracleEffectApplied
			return next, true, ""
		case OracleEffectApplied:
			if e.ResultCode != "applied" && e.ResultCode != "success" && e.ResultCode != "idempotent" {
				return s, false, fmt.Sprintf("reconcile on already applied effect returned %q", e.ResultCode)
			}
			return s, true, ""
		case OracleEffectBlocked:
			if e.ResultCode != "blocked" {
				return s, false, fmt.Sprintf("reconcile on blocked effect returned %q", e.ResultCode)
			}
			return s, true, ""
		default:
			return s, false, fmt.Sprintf("unhandled effect state %q for reconcile operation", s.Effect)
		}
	}

	return s, false, fmt.Sprintf("unknown operation %q in state %+v", e.Operation, s)
}

// LinearizabilityChecker searches for a valid topological execution ordering
// compatible with real-time precedence and the sequential reference model.
type LinearizabilityChecker struct{}

// CheckLinearizability verifies whether the concurrent execution history can be
// linearized into a valid serial sequence.
func (c LinearizabilityChecker) CheckLinearizability(events []HistoryEvent) ([]HistoryEvent, error) {
	if len(events) == 0 {
		return nil, nil
	}
	if len(events) > MaxHistoryEventsBound {
		return nil, fmt.Errorf("history length %d exceeds maximum linearizability bound (%d)", len(events), MaxHistoryEventsBound)
	}

	for _, event := range events {
		if event.EndTime < event.StartTime {
			return nil, fmt.Errorf("event %q ends before it starts (start=%d, end=%d)", event.InvocationID, event.StartTime, event.EndTime)
		}
	}

	// Precedence constraints: if event A finishes before event B starts (EndTime(A) < StartTime(B)),
	// then A must precede B in any valid serialization.
	n := len(events)
	precedes := make([][]bool, n)
	for i := range precedes {
		precedes[i] = make([]bool, n)
	}
	for i := 0; i < n; i++ {
		for j := 0; j < n; j++ {
			if i != j && events[i].EndTime < events[j].StartTime {
				precedes[i][j] = true
			}
		}
	}

	// Backtracking search for a valid topological serialization.
	visited := make([]bool, n)
	order := make([]HistoryEvent, 0, n)

	var search func(lineageStates map[string]OracleModelState) ([]HistoryEvent, bool)
	search = func(lineageStates map[string]OracleModelState) ([]HistoryEvent, bool) {
		if len(order) == n {
			return order, true
		}

		for i := 0; i < n; i++ {
			if visited[i] {
				continue
			}

			// Check real-time precedence: all unvisited predecessors must be satisfied.
			canRun := true
			for p := 0; p < n; p++ {
				if precedes[p][i] && !visited[p] {
					canRun = false
					break
				}
			}
			if !canRun {
				continue
			}

			lineage := events[i].LineageID
			currentState, ok := lineageStates[lineage]
			if !ok {
				currentState = InitialOracleModelState()
			}

			nextState, legal, _ := currentState.Step(events[i])
			if !legal {
				continue
			}

			visited[i] = true
			order = append(order, events[i])

			nextLineageStates := make(map[string]OracleModelState, len(lineageStates)+1)
			for k, v := range lineageStates {
				nextLineageStates[k] = v
			}
			nextLineageStates[lineage] = nextState

			if res, ok := search(nextLineageStates); ok {
				return res, true
			}

			order = order[:len(order)-1]
			visited[i] = false
		}

		return nil, false
	}

	if linearized, ok := search(make(map[string]OracleModelState)); ok {
		return linearized, nil
	}

	return nil, errors.New("history has no legal real-time-compatible serialization in the reference model")
}

// LivenessBounds specifies the bounded execution limits for the oracle release gate.
type LivenessBounds struct {
	MaxTransitionsToTerminal int
	MaxCASAttemptsPerActor   int
}

// DefaultLivenessBounds returns the agreed v1 acceptance bounds.
func DefaultLivenessBounds() LivenessBounds {
	return LivenessBounds{
		MaxTransitionsToTerminal: 4,
		MaxCASAttemptsPerActor:   2,
	}
}

// CheckLiveness verifies that all operations completed within bounded progress constraints.
func (c LinearizabilityChecker) CheckLiveness(events []HistoryEvent, bounds LivenessBounds) error {
	type actorKey struct{ lineage, actor string }
	actorCASAttempts := make(map[actorKey]int)
	lineageTransitions := make(map[string]int)

	for _, e := range events {
		if e.Operation == HistoryOpStart || e.Operation == HistoryOpFinalize {
			lineageTransitions[e.LineageID]++
			if e.ResultCode == "stale_cas" || e.ResultCode == "refused_contention" {
				k := actorKey{lineage: e.LineageID, actor: e.Actor}
				actorCASAttempts[k]++
				if actorCASAttempts[k] > bounds.MaxCASAttemptsPerActor {
					return fmt.Errorf("actor %q in lineage %q exceeded max CAS attempts (%d > %d)", e.Actor, e.LineageID, actorCASAttempts[k], bounds.MaxCASAttemptsPerActor)
				}
			}
		}
	}

	for lineage, count := range lineageTransitions {
		if count > bounds.MaxTransitionsToTerminal {
			return fmt.Errorf("lineage %q transitions (%d) exceeded bounded progress limit (%d)", lineage, count, bounds.MaxTransitionsToTerminal)
		}
	}

	return nil
}

// ScheduleManifestEntry defines a named schedule fixture for the release gate.
type ScheduleManifestEntry struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Actors      int    `json:"actors"`
	Operations  int    `json:"operations"`
}

// CanonicalScheduleManifest returns the finite list of validated release gate schedules.
func CanonicalScheduleManifest() []ScheduleManifestEntry {
	return []ScheduleManifestEntry{
		{
			Name:        "S01-concurrent-start",
			Description: "N identical START callers: one authority/budget, all responses serializable as created or resumed",
			Actors:      3,
			Operations:  3,
		},
		{
			Name:        "S02-concurrent-finalize",
			Description: "N FINALIZE callers at one revision: one commit; losers are idempotent success or stale-CAS",
			Actors:      3,
			Operations:  3,
		},
		{
			Name:        "S03-readonly-gates-during-finalize",
			Description: "N read-only gates during FINALIZE: every allow or retryable result corresponds to one legal revision",
			Actors:      3,
			Operations:  4,
		},
		{
			Name:        "S04-crash-before-effect-publication",
			Description: "Crash after authority commit and before effect publication: STATUS routes to RECONCILE, never START",
			Actors:      2,
			Operations:  3,
		},
		{
			Name:        "S05-concurrent-reconciliation",
			Description: "N reconcilers: exactly one semantic effect and one applied marker",
			Actors:      3,
			Operations:  3,
		},
	}
}

// SortHistoryEvents sorts events by start time for deterministic inspection.
func SortHistoryEvents(events []HistoryEvent) {
	sort.Slice(events, func(i, j int) bool {
		if events[i].StartTime != events[j].StartTime {
			return events[i].StartTime < events[j].StartTime
		}
		if events[i].EndTime != events[j].EndTime {
			return events[i].EndTime < events[j].EndTime
		}
		return events[i].InvocationID < events[j].InvocationID
	})
}
