package strategy

import (
	"math"
	"sync"
	"time"
)

/*
RealizationMeter assesses whether actual account execution can realize the
policy lane's virtual edge. While SkillMeter measures internal policy
profitability on virtual depth, RealizationMeter measures authoritative venue
execution: submission rejections, execution failures, and realized slippage.

It operates as a circuit-breaker / veto key in a two-key execution system:
both policy skill and execution realization must be valid to trade.

Veto semantics are deliberately latching: once tripped by consecutive placement
failures, a catastrophic single-fill slippage event, or sustained adverse EWMA
slippage, execution authority is revoked until an explicit operator Reset()
is invoked.
*/
type RealizationMeter struct {
	lock                 sync.RWMutex
	consecutiveFailures  uint32
	maxFailures          uint32
	totalSubmissions     uint64
	totalFailures        uint64
	totalFills           uint64
	slippageSum          float64
	slippageEwma         float64
	ewmaAlpha            float64
	maxSlippageBps       float64
	maxSingleSlippageBps float64
	vetoed               bool
	vetoReason           string
	vetoTime             time.Time
}

/*
NewRealizationMeter constructs an execution-realization circuit breaker.
It allows trading until authoritative venue feedback trips a veto condition.
*/
func NewRealizationMeter() *RealizationMeter {
	return &RealizationMeter{
		maxFailures:          3,
		maxSlippageBps:       50.0,
		maxSingleSlippageBps: 150.0,
		ewmaAlpha:            0.1,
	}
}

/* AllowsTrading reports whether execution realization currently permits live orders. */
func (meter *RealizationMeter) AllowsTrading() bool {
	if meter == nil {
		return true
	}

	meter.lock.RLock()
	defer meter.lock.RUnlock()

	return !meter.vetoed
}

/* Reason reports why trading is vetoed, or returns an empty string if allowed. */
func (meter *RealizationMeter) Reason() string {
	if meter == nil {
		return ""
	}

	meter.lock.RLock()
	defer meter.lock.RUnlock()

	return meter.vetoReason
}

/* VetoTime reports when the circuit breaker was tripped, or zero time if not vetoed. */
func (meter *RealizationMeter) VetoTime() time.Time {
	if meter == nil {
		return time.Time{}
	}

	meter.lock.RLock()
	defer meter.lock.RUnlock()

	return meter.vetoTime
}

/*
ObserveSubmission records whether the desk accepted a dispatched execution intent.
Consecutive failures tripping the threshold will veto live execution authority.
*/
func (meter *RealizationMeter) ObserveSubmission(err error) {
	if meter == nil {
		return
	}

	meter.lock.Lock()
	defer meter.lock.Unlock()

	meter.totalSubmissions++

	if err != nil {
		meter.consecutiveFailures++
		meter.totalFailures++

		if meter.consecutiveFailures >= meter.maxFailures && !meter.vetoed {
			meter.vetoed = true
			meter.vetoTime = time.Now()
			meter.vetoReason = "consecutive execution submission failures exceeded threshold"
		}

		return
	}

	meter.consecutiveFailures = 0
}

/*
ObserveFill records execution slippage between the intent's reference price and
the venue fill price. A single catastrophic fill or sustained adverse EWMA
slippage trips the latching veto.
*/
func (meter *RealizationMeter) ObserveFill(referencePrice, fillPrice float64, reduce bool) {
	if meter == nil || referencePrice <= 0 || fillPrice <= 0 {
		return
	}

	meter.lock.Lock()
	defer meter.lock.Unlock()

	meter.totalFills++
	slippageBps := 0.0

	if !reduce {
		// Buying: adverse slippage if fillPrice > referencePrice.
		slippageBps = (fillPrice - referencePrice) / referencePrice * 10000.0
	}

	if reduce {
		// Selling: adverse slippage if fillPrice < referencePrice.
		slippageBps = (referencePrice - fillPrice) / referencePrice * 10000.0
	}

	if slippageBps > 0 {
		meter.slippageSum += slippageBps
	}

	// Instantaneous catastrophic bound check
	if slippageBps > meter.maxSingleSlippageBps && !meter.vetoed {
		meter.vetoed = true
		meter.vetoTime = time.Now()
		meter.vetoReason = "catastrophic single-fill slippage exceeded bound"

		return
	}

	// Retained EWMA statistic
	if meter.totalFills == 1 {
		meter.slippageEwma = math.Max(0, slippageBps)
	}

	if meter.totalFills > 1 {
		meter.slippageEwma = meter.ewmaAlpha*math.Max(0, slippageBps) + (1.0-meter.ewmaAlpha)*meter.slippageEwma
	}

	if meter.slippageEwma > meter.maxSlippageBps && !meter.vetoed {
		meter.vetoed = true
		meter.vetoTime = time.Now()
		meter.vetoReason = "realized execution slippage EWMA exceeded tolerance"
	}
}

/* Reset clears a veto and restarts realization counters. */
func (meter *RealizationMeter) Reset() {
	if meter == nil {
		return
	}

	meter.lock.Lock()
	defer meter.lock.Unlock()

	meter.consecutiveFailures = 0
	meter.slippageSum = 0
	meter.slippageEwma = 0
	meter.totalFills = 0
	meter.vetoed = false
	meter.vetoReason = ""
	meter.vetoTime = time.Time{}
}
