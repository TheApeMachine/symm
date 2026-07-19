package trader

import (
	"math/rand/v2"
	"sync"
	"time"

	"github.com/theapemachine/symm/types"
)

/*
reconcilePending re-arms recovered pending intents against the exchange open
orders. Paper (and any non-live transport) answers synchronously so boot tests
and paper runs clear the snapshot on the same tick. Live REST runs off the Tick
goroutine so a slow or failing OpenOrders call cannot freeze the cut loop;
trading stays disabled until that background pass succeeds. Failed live attempts
back off with exponential jitter independent of the inventory tick cadence.
*/
func (crypto *Crypto) reconcilePending() bool {
	snapshot := crypto.snapshot.Load()

	if crypto.api == nil || crypto.desk == nil || snapshot == nil {
		return true
	}

	now := time.Now()

	if !crypto.pendingRetry.Allow(now) {
		return false
	}

	if !crypto.api.Live() {
		return crypto.finishPendingReconcile(snapshot)
	}

	if !crypto.reconcileFlight.CompareAndSwap(false, true) {
		return false
	}

	go func() {
		defer crypto.reconcileFlight.Store(false)
		_ = crypto.finishPendingReconcile(snapshot)
	}()

	return false
}

/*
finishPendingReconcile performs one OpenOrders pass and clears the recovery
snapshot only after a successful desk re-arm.
*/
func (crypto *Crypto) finishPendingReconcile(snapshot *types.Recovery) bool {
	if crypto == nil || crypto.api == nil || crypto.desk == nil {
		return true
	}

	if snapshot == nil {
		return true
	}

	open, err := crypto.api.OpenOrders()

	if err != nil {
		crypto.pendingRetry.Schedule(time.Now())
		return false
	}

	crypto.desk.ReconcilePending(snapshot.PendingOrders, open)
	crypto.pendingRetry.Clear()
	crypto.snapshot.CompareAndSwap(snapshot, nil)
	crypto.trading.Store(true)

	return true
}

const (
	pendingRetryMin = 500 * time.Millisecond
	pendingRetryMax = 30 * time.Second
)

/*
PendingRetry rate-limits failed OpenOrders reconciliation independently of the
inventory tick cadence so REST failures do not hammer the venue every sync.
*/
type PendingRetry struct {
	mu      sync.Mutex
	at      time.Time
	backoff time.Duration
}

/*
Allow reports whether an OpenOrders attempt may proceed at now.
*/
func (retry *PendingRetry) Allow(now time.Time) bool {
	if retry == nil {
		return true
	}

	retry.mu.Lock()
	defer retry.mu.Unlock()

	if retry.at.IsZero() {
		return true
	}

	return !now.Before(retry.at)
}

/*
Schedule advances the backoff window with exponential growth and ±25% jitter.
*/
func (retry *PendingRetry) Schedule(now time.Time) {
	if retry == nil {
		return
	}

	retry.mu.Lock()
	defer retry.mu.Unlock()

	if retry.backoff <= 0 {
		retry.backoff = pendingRetryMin
	} else {
		retry.backoff *= 2

		if retry.backoff > pendingRetryMax {
			retry.backoff = pendingRetryMax
		}
	}

	span := retry.backoff
	jitter := time.Duration(float64(span) * (0.75 + 0.5*rand.Float64()))
	retry.at = now.Add(jitter)
}

/*
Clear resets backoff after a successful reconcile.
*/
func (retry *PendingRetry) Clear() {
	if retry == nil {
		return
	}

	retry.mu.Lock()
	defer retry.mu.Unlock()

	retry.at = time.Time{}
	retry.backoff = 0
}
