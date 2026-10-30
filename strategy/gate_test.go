package strategy

import (
	"testing"

	"github.com/theapemachine/nomagique/data"
	nmstore "github.com/theapemachine/nomagique/store"
)

/*
newTestGate builds a Gate with no desk/price/store so Evaluate can exercise the
hurdle math alone. The store being nil means persist() is a no-op.
*/
func newTestGate(t *testing.T) *Gate {
	return NewGate(
		t.Context(),
		nil,
		nil,
		nil,
	)
}

func region(key uint8, brightness float64) *nmstore.Region {
	return &nmstore.Region{Key: key, Brightness: brightness}
}

// --- Issue #3: the gate is a bounded, correctly-signed regressor ---

func TestGateBoundedAndBoundedByConfig(t *testing.T) {
	gate := newTestGate(t)

	for range 1000 {
		gate.Update(GateResult{ReturnFraction: -1.0}) // worst-case loss, repeated
	}

	// Each hurdle must stay inside its configured band, never random-walk to zero
	// or infinity.
	if gate.ignition < gate.ignitionFloor-1e-9 || gate.ignition > gate.ignitionCeil+1e-9 {
		t.Fatalf("ignition %v escaped [%v,%v]", gate.ignition, gate.ignitionFloor, gate.ignitionCeil)
	}
	if gate.risk < gate.riskFloor-1e-9 || gate.risk > gate.riskCeil+1e-9 {
		t.Fatalf("risk %v escaped [%v,%v]", gate.risk, gate.riskFloor, gate.riskCeil)
	}
	if gate.rejection < gate.rejectionFloor-1e-9 || gate.rejection > gate.rejectionCeil+1e-9 {
		t.Fatalf("rejection %v escaped [%v,%v]", gate.rejection, gate.rejectionFloor, gate.rejectionCeil)
	}
}

func TestGateWinLowersIgnitionRisk(t *testing.T) {
	gate := newTestGate(t)
	beforeIgnition, beforeRisk := gate.ignition, gate.risk

	gate.Update(GateResult{ReturnFraction: +0.5})

	if gate.ignition >= beforeIgnition-1e-9 {
		t.Fatalf("a win should lower the ignition floor: %v !< %v", gate.ignition, beforeIgnition)
	}
	if gate.risk >= beforeRisk-1e-9 {
		t.Fatalf("a win should tighten risk: %v !< %v", gate.risk, beforeRisk)
	}
}

func TestGateLossRaisesIgnitionRisk(t *testing.T) {
	gate := newTestGate(t)
	gate.ignition = 0.5
	beforeIgnition, beforeRisk := gate.ignition, gate.risk

	gate.Update(GateResult{ReturnFraction: -0.5})

	if gate.ignition <= beforeIgnition+1e-9 {
		t.Fatalf("a loss should raise the ignition floor: %v !> %v", gate.ignition, beforeIgnition)
	}
	if gate.risk <= beforeRisk+1e-9 {
		t.Fatalf("a loss should raise risk tolerance: %v !> %v", gate.risk, beforeRisk)
	}
}

/*
This is the regression the whole #3 is about: a harvested win must RAISE the
rejection (take-profit) hurdle, not lower it. The old exp(-return) product did
the opposite and clipped the best trades.
*/
func TestGateWinRaisesRejection(t *testing.T) {
	gate := newTestGate(t)
	before := gate.rejection

	gate.Update(GateResult{ReturnFraction: +0.5, StoppedOut: false})

	if gate.rejection <= before+1e-9 {
		t.Fatalf("a harvested win must raise the rejection hurdle: %v !> %v (winners were being clipped)",
			gate.rejection, before)
	}
}

func TestGateStopOutLowersRejection(t *testing.T) {
	gate := newTestGate(t)
	before := gate.rejection

	gate.Update(GateResult{ReturnFraction: -0.5, StoppedOut: true})

	if gate.rejection >= before-1e-9 {
		t.Fatalf("a stop-out must lower the rejection hurdle (take profits sooner): %v !< %v",
			gate.rejection, before)
	}
}

func TestGateNoFallbackToZeroOnFlat(t *testing.T) {
	gate := newTestGate(t)
	before := gate.rejection
	gate.Update(GateResult{ReturnFraction: 0})
	if gate.rejection != before {
		t.Fatalf("a flat outcome must not move the rejection: %v -> %v", before, gate.rejection)
	}
}

// --- Issue #5: opposition is risk (a gate input), never a subtractor from impulse ---

/*
A contested breakout: detector + support are bright AND opposition is bright.
The gate must not demote it to Virtual on the strength of opposition alone;
opposition is the risk input that raises the risk-to-net-spark test, not a
force that cancels the spark. With net spark positive the gate permits Paper.
*/
func TestGateNegativeEnterNetBlocks(t *testing.T) {
	gate := newTestGate(t)
	decision := gate.EvaluateEnter("BTC/USD", []*nmstore.Region{
		region(0, 0.2),
		region(1, 0.2),
		region(2, 0.9),
	}, CoderReading{Ready: true, Bullish: true})

	if decision.Allowed {
		t.Fatalf("opposition-led enter net must block, got %+v", decision)
	}
}

func TestGateContestedBreakoutNotCancelledByOpposition(t *testing.T) {
	gate := newTestGate(t)
	// Force the ignition hurdle below the ignition we present, and the risk
	// tolerance above the risk-to-net-spark we present, so the *only* question
	// is the sign of the net spark.
	gate.ignition = 0.1
	gate.risk = 5.0

	observed := []*nmstore.Region{
		region(0, 0.9), // detector
		region(1, 0.9), // support
		region(2, 0.6), // opposition: bright wall
		region(6, 0.9), // hold ignition regime
		region(7, 0.9), // ignition clears the 0.1 floor
		region(12, 0.1),
	}

	decision := gate.EvaluateEnter("BTC/USD", observed, CoderReading{Ready: true, Bullish: true})

	if !decision.Allowed {
		t.Fatalf("contested breakout must be allowed, got %q", decision.Reason)
	}
	// net spark = (0.9+0.9)-0.6 = 1.2 > 0, risk 0 -> risk-dominant false -> Paper.
	if decision.Virtual {
		t.Fatalf("a breakout with positive net spark must earn Paper, got virtual: %q", decision.Reason)
	}
}

/*
The same setup but with opposition dominating the spark: the gate demotes to
Virtual (it allows the trade but sandboxes it), it never blocks it outright.
*/
func TestGateRiskDominanceDemotesToVirtual(t *testing.T) {
	gate := newTestGate(t)
	gate.ignition = 0.1
	gate.risk = 0.3 // tight risk tolerance
	gate.regimeFriction = 5.0

	observed := []*nmstore.Region{
		region(0, 0.9),  // detector
		region(1, 0.8),  // support
		region(2, 0.4),  // opposition present but enter net stays positive
		region(6, 0.9),  // ignition (when require_ignition_regime is on)
		region(7, 0.9),
		region(12, 2.5), // risk oracle dominates net spark
	}

	decision := gate.EvaluateEnter("BTC/USD", observed, CoderReading{Ready: true, Bullish: true})

	if !decision.Allowed {
		t.Fatalf("risk dominance demotes to Virtual, never blocks; got blocked %q", decision.Reason)
	}
	if !decision.Virtual {
		t.Fatalf("risk-dominant setup must be Virtual, got paper: %q", decision.Reason)
	}
}

/*
A calibrated non-bullish coder must not earn paper; it may still virtual-explore.
*/
func TestGateNonBullishCoderVirtualOnly(t *testing.T) {
	gate := newTestGate(t)

	decision := gate.EvaluateEnter("BTC/USD", []*nmstore.Region{region(0, 1), region(7, 1)}, CoderReading{Ready: true, Bullish: false})

	if !decision.Allowed || !decision.Virtual {
		t.Fatalf("a calibrated non-bullish coder must allow virtual only, got %+v", decision)
	}
}

// --- Issue #8 / #20: an uncalibrated coder permits virtual entry for learning ---

func TestGateUncalibratedCoderVirtual(t *testing.T) {
	gate := newTestGate(t)
	decision := gate.EvaluateEnter("BTC/USD", []*nmstore.Region{region(0, 1), region(7, 1)}, CoderReading{})
	if !decision.Allowed || !decision.Virtual {
		t.Fatalf("an uncalibrated coder must allow virtual entry, got %+v", decision)
	}
}

func TestGateUpdateRegimeHurdlesFromVirtualRoundTrip(t *testing.T) {
	gate := newTestGate(t)
	gate.opposition = 0.5
	gate.maturityMin = 0.5
	gate.regimeIgnition = 0.5
	gate.regimeFriction = 0.5

	gate.Update(GateResult{
		ReturnFraction: -0.1,
		Virtual:        true,
		EnterR02:       0.6,
		EnterNetHold:   0.1,
		GridMaturity:   0.4,
	})

	gate.mu.RLock()
	defer gate.mu.RUnlock()
	if gate.opposition >= 0.5 {
		t.Fatalf("loss at weak hold should lower opposition hurdle, got %v", gate.opposition)
	}
	if gate.maturityMin <= 0.5 {
		t.Fatalf("virtual loss should raise maturity hurdle, got %v", gate.maturityMin)
	}
	if gate.regimeIgnition <= 0.5 {
		t.Fatalf("virtual loss should raise regime ignition hurdle, got %v", gate.regimeIgnition)
	}
	if gate.regimeFriction >= 0.5 {
		t.Fatalf("virtual loss should lower friction hurdle (tighter R12 gate), got %v", gate.regimeFriction)
	}
}

func TestGateEnterOppositionIgnitionConflictBlocks(t *testing.T) {
	gate := newTestGate(t)
	gate.ignition = 1.0

	observed := []*nmstore.Region{
		region(0, 0.5),
		region(1, 0.5),
		region(9, 0.5),
		region(2, 0.5),
		region(6, 0.1),
		region(7, 0.1),
		region(8, 0.2),
	}

	decision := gate.EvaluateEnter("DCR/USD", observed, CoderReading{Ready: true, Bullish: true})

	if decision.Allowed {
		t.Fatalf("bright enter opposition with dead hold ignition must block, got %+v", decision)
	}
}

// --- Issue #8: populate Confidence from the forward curve so it is load-bearing ---

func TestReadCoderPopulatesConfidence(t *testing.T) {
	tick := data.NewValue("training", "step", "BTC/USD")
	tick.Append([]float64{1}, "resonance", "BTC-USD", "calibrated")
	tick.Append([]float64{15}, "resonance", "BTC-USD", "supported_horizon")
	tick.Append([]float64{1.2}, "resonance", "BTC-USD", "task_relative_precision")
	for h := 0; h < 15; h++ {
		z := 0.25
		if h == 2 || h == 5 || h == 8 {
			z = 0.05
		}
		if h >= 12 {
			z = 0.1
		}
		tick.Append([]float64{z}, "resonance", "BTC-USD", "forward_curve_"+itoa(h))
	}

	reading := ReadCoder(tick, "BTC/USD")

	if !reading.Bullish {
		t.Fatalf("a clean forward curve should be bullish, got %+v", reading)
	}
	// Confidence must be populated and positive: it scales enter impulse and
	// arms the exit veto. A zero confidence would make both a silent no-op.
	if reading.Confidence <= 0 {
		t.Fatalf("a bullish coder must carry positive confidence, got %v", reading.Confidence)
	}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	neg := n < 0
	if neg {
		n = -n
	}
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}
