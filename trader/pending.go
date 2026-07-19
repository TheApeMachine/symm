package trader

import (
	"math/rand/v2"
	"time"
)

/*
reconcilePending re-arms recovered pending intents against the exchange open
orders. Paper answers with an empty set (synchronous fills), which is a
successful reconcile; a live REST failure returns false so the caller keeps the
snapshot and trading stays disabled. Failed OpenOrders calls back off with
exponential jitter tracked independently of the syncInventory tick cadence.
*/
func (crypto *Crypto) reconcilePending() bool {
	if crypto.api == nil || crypto.desk == nil || crypto.snapshot == nil {
		return true
	}

	now := time.Now()

	if !crypto.pendingRetry.Allow(now) {
		return false
	}

	open, err := crypto.api.OpenOrders()

	if err != nil {
		crypto.pendingRetry.Schedule(now)
		return false
	}

	crypto.desk.ReconcilePending(crypto.snapshot.PendingOrders, open)
	crypto.pendingRetry.Clear()

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
	at      time.Time
	backoff time.Duration
}

/*
Allow reports whether an OpenOrders attempt may proceed at now.
*/
func (retry *PendingRetry) Allow(now time.Time) bool {
	if retry == nil || retry.at.IsZero() {
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

	retry.at = time.Time{}
	retry.backoff = 0
}
