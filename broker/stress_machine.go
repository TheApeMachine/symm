package broker

import (
	"fmt"
	"math"
	"sync"
	"time"
)

type stressPhase int

const (
	stressHealthy stressPhase = iota
	stressDegraded
	stressOutage
)

const (
	stressDegradedThreshold = 2.0
	stressOutageThreshold   = 4.0
	stressRecoverThreshold  = 1.0
	stressOutageDwell       = 5 * time.Second
	stressDegradedLatency   = 200 * time.Millisecond
	stressOutageLatency     = 5 * time.Second
	stressDegradedRejectMul = 2.0
)

/*
StressMachine models clustered exchange outages as a Markov chain instead of
independent Bernoulli draws per order.
*/
type StressMachine struct {
	mu          sync.Mutex
	phase       stressPhase
	phaseUntil  time.Time
	lastAdvance time.Time
}

var globalStressMachine StressMachine

/*
GlobalStressMachine returns the shared paper execution stress state machine.
*/
func GlobalStressMachine() *StressMachine {
	return &globalStressMachine
}

/*
ResetStressMachine clears outage state between replay eval subprocesses.
*/
func ResetStressMachine() {
	globalStressMachine.mu.Lock()
	defer globalStressMachine.mu.Unlock()

	globalStressMachine.phase = stressHealthy
	globalStressMachine.phaseUntil = time.Time{}
	globalStressMachine.lastAdvance = time.Time{}
}

/*
Advance transitions the outage chain from live microstructure intensity.
*/
func (machine *StressMachine) Advance(regime StressRegime, now time.Time) {
	machine.mu.Lock()
	defer machine.mu.Unlock()

	if now.IsZero() {
		now = time.Now()
	}

	intensity := math.Max(regime.Turbulence, math.Abs(regime.Vorticity))
	machine.lastAdvance = now

	if !machine.phaseUntil.IsZero() && now.Before(machine.phaseUntil) {
		return
	}

	switch machine.phase {
	case stressOutage:
		machine.phase = stressDegraded
		machine.phaseUntil = time.Time{}

		return
	case stressDegraded:
		if intensity >= stressOutageThreshold && machine.shouldTransition(0.35) {
			machine.enterOutage(now)

			return
		}

		if intensity < stressRecoverThreshold {
			machine.phase = stressHealthy
			machine.phaseUntil = time.Time{}
		}

		return
	default:
		if intensity >= stressOutageThreshold && machine.shouldTransition(0.2) {
			machine.enterOutage(now)

			return
		}

		if intensity >= stressDegradedThreshold {
			machine.phase = stressDegraded
			machine.phaseUntil = time.Time{}
		}
	}
}

func (machine *StressMachine) enterOutage(now time.Time) {
	machine.phase = stressOutage
	machine.phaseUntil = now.Add(stressOutageDwell)
}

func (machine *StressMachine) shouldTransition(probability float64) bool {
	if probability <= 0 {
		return false
	}

	draw, err := cryptoFloat64()

	if err != nil {
		return false
	}

	return draw < probability
}

/*
Phase reports the current outage chain state for tests and telemetry.
*/
func (machine *StressMachine) Phase() string {
	machine.mu.Lock()
	defer machine.mu.Unlock()

	switch machine.phase {
	case stressDegraded:
		return "degraded"
	case stressOutage:
		return "outage"
	default:
		return "healthy"
	}
}

/*
RejectOutcome returns an error when the current phase simulates an exchange reject.
*/
func (machine *StressMachine) RejectOutcome(baseRate float64, regime StressRegime) error {
	machine.mu.Lock()
	phase := machine.phase
	machine.mu.Unlock()

	switch phase {
	case stressOutage:
		return fmt.Errorf("execution stress outage reject (phase=outage)")
	case stressDegraded:
		rate := math.Min(1, baseRate*regime.Multiplier()*stressDegradedRejectMul)

		return bernoulliReject(rate, "execution stress degraded reject")
	default:
		rate := EffectiveRejectRate(baseRate, regime)

		return bernoulliReject(rate, "execution stress reject")
	}
}

/*
LatencyPenalty returns quote-age stress for the current outage phase.
*/
func (machine *StressMachine) LatencyPenalty(baseLatency time.Duration, regime StressRegime) time.Duration {
	machine.mu.Lock()
	phase := machine.phase
	machine.mu.Unlock()

	switch phase {
	case stressOutage:
		return stressOutageLatency
	case stressDegraded:
		scaled := EffectiveStressLatency(baseLatency, regime)

		if scaled < stressDegradedLatency {
			return stressDegradedLatency
		}

		return scaled
	default:
		return EffectiveStressLatency(baseLatency, regime)
	}
}

func bernoulliReject(rate float64, label string) error {
	if rate <= 0 {
		return nil
	}

	draw, err := cryptoFloat64()

	if err != nil {
		return fmt.Errorf("%s entropy: %w", label, err)
	}

	if draw < rate {
		return fmt.Errorf("%s (rate=%.4f)", label, rate)
	}

	return nil
}
