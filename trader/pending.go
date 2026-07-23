package trader

import (
	"context"
	"sync/atomic"
)

/*
pending counts ingress frames still inside signal Measure so SyncTick can
barrier on Calculate completion without sleeping.
*/
type pending struct {
	count atomic.Int64
	ping  chan struct{}
}

/*
newPending constructs a zeroed ingress barrier.
*/
func newPending() *pending {
	return &pending{ping: make(chan struct{}, 1)}
}

/*
Add adjusts the in-flight ingress count and wakes waiters when it returns to zero.
*/
func (pending *pending) Add(delta int64) {
	if pending.count.Add(delta) != 0 {
		return
	}

	select {
	case pending.ping <- struct{}{}:
	default:
	}
}

/*
Wait blocks until every counted ingress frame has been acknowledged.
*/
func (pending *pending) Wait(ctx context.Context) error {
	for pending.count.Load() > 0 {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-pending.ping:
		}
	}

	return nil
}
