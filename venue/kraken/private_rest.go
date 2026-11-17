package kraken

import (
	"context"
	"strings"
	"sync"
	"time"

	"github.com/spf13/viper"
	"github.com/theapemachine/errnie"
)

/*
Spot REST call-counter tiers from
https://docs.kraken.com/exchange/guides/rest/ratelimits

Ledger / trade history: +2. Every other REST call: +1.
EAPI:Rate limit exceeded means the counter is past Max — further calls while
limited extend the restriction, so PrivateREST refuses new Holds until decay
creates headroom again.
*/
const (
	TierStarter       = "starter"
	TierIntermediate  = "intermediate"
	TierPro           = "pro"
	restHistoryCost   = 2.0
	restCallCost      = 1.0
)

/*
RESTTier is one verification row of the REST call counter table.
*/
type RESTTier struct {
	Name  string
	Max   float64
	Decay float64 // counter units per second
}

/*
PrivateREST serializes signed spot REST on one API key and mirrors Kraken's
call counter so Balance, TradeBalance, TradesHistory, and GetWebSocketsToken
share one budget. Trading-engine (EOrder) limits are per-pair and live elsewhere.
*/
type PrivateREST struct {
	mu        sync.Mutex
	now       func() time.Time
	counter   float64
	lastDecay time.Time
	throttle  time.Time
}

/*
Private is the process-wide spot REST gate. Nonce signing and the counter both
require exclusive access on the same key.
*/
var Private = NewPrivateREST()

/*
NewPrivateREST starts at counter 0 with wall clock, matching a fresh API key
session.
*/
func NewPrivateREST() *PrivateREST {
	return &PrivateREST{now: time.Now}
}

/*
Cost is the REST counter increment for url. TradesHistory and Ledgers are +2;
everything else is +1.
*/
func (rest *PrivateREST) Cost(url string) float64 {
	lower := strings.ToLower(url)

	if strings.Contains(lower, "tradeshistory") || strings.Contains(lower, "/ledgers") {
		return restHistoryCost
	}

	return restCallCost
}

/*
Hold blocks until the local counter has headroom for cost (and any
EService:Throttled deadline has passed), then leaves the mutex held. Caller
must Release. Commit after a venue response so dial failures do not inflate
the counter.
*/
func (rest *PrivateREST) Hold(ctx context.Context, cost float64) error {
	if rest == nil {
		return errnie.Error(errnie.Err(
			errnie.Validation, "[kraken.private_rest] nil gate", nil,
		))
	}

	if cost <= 0 {
		cost = restCallCost
	}

	if ctx == nil {
		ctx = context.Background()
	}

	for {
		if err := ctx.Err(); err != nil {
			return errnie.Error(errnie.Err(
				errnie.IO, "[kraken.private_rest] hold cancelled", err,
			))
		}

		rest.mu.Lock()
		wait := rest.waitNeededLocked(cost)

		if wait <= 0 {
			return nil
		}

		rest.mu.Unlock()

		if !sleepContext(ctx, wait) {
			return errnie.Error(errnie.Err(
				errnie.IO, "[kraken.private_rest] hold cancelled", ctx.Err(),
			))
		}
	}
}

/*
Release drops the mutex taken by Hold.
*/
func (rest *PrivateREST) Release() {
	if rest == nil {
		return
	}

	rest.mu.Unlock()
}

/*
Commit adds cost after the venue has accepted the request (HTTP response or
SDK result). Must run while Hold is active.
*/
func (rest *PrivateREST) Commit(cost float64) {
	if rest == nil {
		return
	}

	if cost <= 0 {
		cost = restCallCost
	}

	rest.decayLocked()
	rest.counter += cost
}

/*
Observe folds a Kraken error array into the gate: throttle deadlines and
rate-limit snaps. Must run while Hold is active (same as Commit).
*/
func (rest *PrivateREST) Observe(errs ...any) {
	if rest == nil || len(errs) == 0 {
		return
	}

	if until, ok := ThrottleUntil(errs...); ok {
		rest.tripThrottleLocked(until)
	}

	if RateLimited(errs...) {
		rest.tripRateLimitLocked()
	}
}

/*
ObserveError folds SDK / transport errors that carry EAPI:Rate limit exceeded
or EService:Throttled without a JSON body. Must run while Hold is active.
*/
func (rest *PrivateREST) ObserveError(err error) {
	if rest == nil || err == nil {
		return
	}

	rest.Observe(err)
}

/*
Waiting is how long a reconnect loop should sleep before the next private
REST attempt (throttle deadline or decay to one call of headroom).
*/
func (rest *PrivateREST) Waiting() time.Duration {
	if rest == nil {
		return 0
	}

	rest.mu.Lock()
	defer rest.mu.Unlock()

	return rest.waitNeededLocked(restCallCost)
}

func (rest *PrivateREST) waitNeededLocked(cost float64) time.Duration {
	rest.decayLocked()
	now := rest.clock()

	if !rest.throttle.IsZero() && now.Before(rest.throttle) {
		return rest.throttle.Sub(now)
	}

	tier := rest.tier()

	if rest.counter+cost <= tier.Max {
		return 0
	}

	deficit := rest.counter + cost - tier.Max

	return time.Duration(deficit / tier.Decay * float64(time.Second))
}

func (rest *PrivateREST) decayLocked() {
	now := rest.clock()

	if rest.lastDecay.IsZero() {
		rest.lastDecay = now
		return
	}

	elapsed := now.Sub(rest.lastDecay).Seconds()

	if elapsed <= 0 {
		return
	}

	rest.counter -= rest.tier().Decay * elapsed

	if rest.counter < 0 {
		rest.counter = 0
	}

	rest.lastDecay = now
}

func (rest *PrivateREST) tripRateLimitLocked() {
	rest.decayLocked()
	tier := rest.tier()

	if rest.counter < tier.Max {
		rest.counter = tier.Max
	}
}

func (rest *PrivateREST) tripThrottleLocked(until time.Time) {
	if until.IsZero() {
		return
	}

	if until.After(rest.throttle) {
		rest.throttle = until.UTC()
	}
}

func (rest *PrivateREST) clock() time.Time {
	if rest.now != nil {
		return rest.now()
	}

	return time.Now()
}

func (rest *PrivateREST) tier() RESTTier {
	return ParseRESTTier(viper.GetString("kraken.verification_tier"))
}

/*
ParseRESTTier maps a verification tier name to the published max and decay.
Unknown names fall back to Intermediate — the desk's documented steady-state
assumption for Balance + TradeBalance at private_refresh.
*/
func ParseRESTTier(name string) RESTTier {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case TierStarter:
		return RESTTier{Name: TierStarter, Max: 15, Decay: 0.33}
	case TierPro:
		return RESTTier{Name: TierPro, Max: 20, Decay: 1}
	default:
		return RESTTier{Name: TierIntermediate, Max: 20, Decay: 0.5}
	}
}

func sleepContext(ctx context.Context, wait time.Duration) bool {
	if wait <= 0 {
		return ctx.Err() == nil
	}

	timer := time.NewTimer(wait)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}

func init() {
	viper.SetDefault("kraken.verification_tier", TierIntermediate)
}
