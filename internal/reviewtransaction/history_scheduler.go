package reviewtransaction

import "context"

type HistoryPoint string

const (
	HistoryBeforeLock       HistoryPoint = "before_lock"
	HistoryAfterLock        HistoryPoint = "after_lock"
	HistoryAfterRead        HistoryPoint = "after_authority_read"
	HistoryBeforeCAS        HistoryPoint = "before_cas"
	HistoryAfterCommit      HistoryPoint = "after_authority_commit"
	HistoryBeforeEffectRead HistoryPoint = "before_effect_observe"
	HistoryBeforeEffect     HistoryPoint = "before_effect_write"
	HistoryBeforeMarker     HistoryPoint = "before_marker_cas"
	HistoryAfterMarker      HistoryPoint = "after_marker_cas"
	HistoryBeforeResponse   HistoryPoint = "before_response"
)

type HistoryBoundary struct {
	Operation string
	LineageID string
	Revision  string
}

type HistoryScheduler interface {
	Reach(context.Context, string, HistoryPoint, HistoryBoundary) error
}

type historySchedulerKey struct{}

func WithHistoryScheduler(ctx context.Context, scheduler HistoryScheduler) context.Context {
	if scheduler == nil {
		return ctx
	}
	return context.WithValue(ctx, historySchedulerKey{}, scheduler)
}

func reachHistory(ctx context.Context, actor string, point HistoryPoint, boundary HistoryBoundary) error {
	if ctx == nil {
		ctx = context.Background()
	}
	scheduler, _ := ctx.Value(historySchedulerKey{}).(HistoryScheduler)
	if scheduler == nil {
		return nil
	}
	return scheduler.Reach(ctx, actor, point, boundary)
}
