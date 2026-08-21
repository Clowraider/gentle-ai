// File history_scheduler.go — deterministic history scheduler for review authority
// transitions (Issue #1874 bounded v1).

package reviewtransaction

import "context"

// HistoryPoint identifies a deterministic synchronization point during review
// authority transitions.
type HistoryPoint string

const (
	HistoryPointAfterAuthorityRead   HistoryPoint = "after_authority_read"
	HistoryPointBeforeLock           HistoryPoint = "before_lock"
	HistoryPointAfterLock            HistoryPoint = "after_lock"
	HistoryPointBeforeCAS            HistoryPoint = "before_cas"
	HistoryPointAfterCAS             HistoryPoint = "after_cas"
	HistoryPointAfterAuthorityCommit HistoryPoint = "after_authority_commit"
	HistoryPointBeforeEffectObserve  HistoryPoint = "before_effect_observe"
	HistoryPointBeforeEffectWrite    HistoryPoint = "before_effect_write"
	HistoryPointAfterEffectWrite     HistoryPoint = "after_effect_write"
	HistoryPointBeforeMarkerCAS      HistoryPoint = "before_marker_cas"
	HistoryPointAfterMarkerCAS       HistoryPoint = "after_marker_cas"
	HistoryPointBeforeResponse       HistoryPoint = "before_response"
)

// HistoryScheduler coordinates deterministic interleavings and fault injection
// across concurrent actors during review authority operations.
type HistoryScheduler interface {
	Reach(ctx context.Context, actor string, point HistoryPoint, metadata any) error
}

type noopHistoryScheduler struct{}

func (noopHistoryScheduler) Reach(_ context.Context, _ string, _ HistoryPoint, _ any) error {
	return nil
}

type historySchedulerContextKey struct{}

// WithHistoryScheduler returns a context carrying the provided HistoryScheduler.
func WithHistoryScheduler(ctx context.Context, scheduler HistoryScheduler) context.Context {
	if scheduler == nil {
		return ctx
	}
	return context.WithValue(ctx, historySchedulerContextKey{}, scheduler)
}

// HistorySchedulerFromContext extracts the HistoryScheduler from context, or
// returns a no-op implementation.
func HistorySchedulerFromContext(ctx context.Context) HistoryScheduler {
	if ctx == nil {
		return noopHistoryScheduler{}
	}
	if s, ok := ctx.Value(historySchedulerContextKey{}).(HistoryScheduler); ok && s != nil {
		return s
	}
	return noopHistoryScheduler{}
}
