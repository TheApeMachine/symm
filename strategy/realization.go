package strategy

import (
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
*/
type RealizationMeter struct {
	lock                sync.RWMutex
	consecutiveFailures uint32
	maxFailures         uint32
	totalSubmissions    uint64
	totalFailures       uint64
	totalFills          uint64
	slippageSum         float64
	maxSlippageBps      float64
	vetoed              bool
	vetoReason          string
	vetoTime            time.Time
}

/*
NewRealizationMeter constructs an execution-realization circuit breaker.
It allows trading until authoritative venue feedback trips a veto condition.
*/
func NewRealizationMeter() *RealizationMeter {
	return &RealizationMeter{
		maxFailures:    3,
		maxSlippageBps: 50.0,
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
the venue fill price. Adverse slippage exceeding the allowed budget trips a veto.
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

	averageSlippage := meter.slippageSum / float64(meter.totalFills)

	if averageSlippage > meter.maxSlippageBps && !meter.vetoed {
		meter.vetoed = true
		meter.vetoTime = time.Now()
		meter.vetoReason = "realized execution slippage exceeded tolerance"
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
	meter.totalFills = 0
	meter.vetoed = false
	meter.vetoReason = ""
	meter.vetoTime = time.Time{}
}
