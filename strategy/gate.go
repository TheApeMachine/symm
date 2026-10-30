package strategy

import (
	"context"
	"math"

	"github.com/theapemachine/nomagique/data"
	nmstore "github.com/theapemachine/nomagique/store"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/store"
	"github.com/theapemachine/symm/system"
)

// GateDecision conveys whether an action can proceed, and if it must be Virtual.
type GateDecision struct {
	Allowed bool   // false = blocked by gate
	Virtual bool   // true = sandbox only, false = permitted for Paper
	Reason  string // diagnostic reason
}

/*
Gate learns three environmental hurdles from realized outcomes and bounds each
to a config [floor, ceil]. On every settled round trip each hurdle takes one
bounded step toward the target the outcome implies. The targets are signed so
the learning direction is unambiguous:

	ignition  (a MINIMUM to earn Paper):  a win lowers it (easier), a loss raises it.
	risk      (a MAXIMUM tolerated):      a win lowers it (tighter), a loss raises it.
	rejection (a MINIMUM force to take a profit): a harvested win RAISES it (hold
	        longer); a stop-out lowers it (take profits sooner).

The previous exp(-return) product was a stateless random walk that also clipped
winners (a good harvest lowered the take-profit force). Regressing to a signed,
bounded target removes both.
*/
type Gate struct {
	ctx   context.Context
	desk  *broker.Desk
	price *broker.Price
	store *store.Client

	ignition  float64
	risk      float64
	rejection float64

	ignitionFloor, ignitionCeil   float64
	riskFloor, riskCeil           float64
	rejectionFloor, rejectionCeil float64
	rate                          float64
}

// GateResult is the settled round trip the gate learns from: the realized
// fee-net return and how the position exited.
type GateResult struct {
	ReturnFraction float64 // realized fee-net fraction of cost (+win / -loss)
	StoppedOut     bool    // exited by an invalidation/evaporation stop, not a harvest
}

func NewGate(
	ctx context.Context,
	desk *broker.Desk,
	price *broker.Price,
	client *store.Client,
) *Gate {
	rate := 0.10
	ignitionFloor, ignitionCeil := 0.15, 1.0
	riskFloor, riskCeil := 0.5, 5.0
	rejectionFloor, rejectionCeil := 0.3, 4.0

	if cfg := system.Cfg.Learning; cfg != nil {
		if cfg.GateHurdleRate > 0 {
			rate = cfg.GateHurdleRate
		}
		ignitionFloor, ignitionCeil = cfg.GateIgnitionFloor, cfg.GateIgnitionCeil
		riskFloor, riskCeil = cfg.GateRiskFloor, cfg.GateRiskCeil
		rejectionFloor, rejectionCeil = cfg.GateRejectionFloor, cfg.GateRejectionCeil
	}

	gate := &Gate{
		ctx:            ctx,
		desk:           desk,
		price:          price,
		store:          client,
		ignition:       1.0,
		risk:           1.0,
		rejection:      1.0,
		ignitionFloor:  ignitionFloor,
		ignitionCeil:   ignitionCeil,
		riskFloor:      riskFloor,
		riskCeil:       riskCeil,
		rejectionFloor: rejectionFloor,
		rejectionCeil:  rejectionCeil,
		rate:           rate,
	}

	gate.restore()
	return gate
}

func (gate *Gate) restore() {
	if gate == nil || gate.store == nil {
		return
	}

	value := gate.store.Download(gate.ctx, "model/parameters/gate.bin")
	if value == nil || value.Err != nil {
		return
	}

	if h := value.GetFloat64("ignition_hurdle"); h > 0 {
		gate.ignition = h
	}
	if r := value.GetFloat64("risk_tolerance"); r > 0 {
		gate.risk = r
	}
	if rej := value.GetFloat64("rejection_hurdle"); rej > 0 {
		gate.rejection = rej
	}
}

func (gate *Gate) persist() {
	if gate.store == nil {
		return
	}

	value := data.NewValue("model", "parameters", "gate")
	value.Set(gate.ignition, "ignition_hurdle")
	value.Set(gate.risk, "risk_tolerance")
	value.Set(gate.rejection, "rejection_hurdle")
	gate.store.WriteAtPrefix("model/parameters/gate.bin", value.Bytes())
}

func clamp(v, floor, ceil float64) float64 {
	if v < floor {
		return floor
	}
	if v > ceil {
		return ceil
	}
	return v
}

/*
EvaluateEnter inspects entry invariants and environmental posture.
Physical impossibilities block entirely; environmental doubts relegate to Virtual.
*/
func (gate *Gate) EvaluateEnter(
	symbol string,
	observed []*nmstore.Region,
	coder CoderReading,
) GateDecision {
	forceVirtual := false
	if !coder.Ready && !coder.Bullish {
		forceVirtual = true
	} else if !coder.Bullish {
		return GateDecision{Allowed: false, Reason: "coder not bullish or uncalibrated"}
	}

	if gate.desk != nil && !gate.desk.CanSize(symbol) {
		return GateDecision{Allowed: false, Reason: "insufficient book liquidity (CanSize failed)"}
	}

	// PHYSICAL INVARIANT: Anti-Hollow Book Filter (R12 Friction Hurdle)
	if gate.desk != nil {
		if spreadPct, ok := gate.price.SpreadPct(symbol); ok && spreadPct != nil {
			if spreadPct.Float64() > 4.0 {
				return GateDecision{Allowed: false, Reason: "blown-out spread: bid-ask spread exceeds 4.0%"}
			}
		}

		if dragBudget, ok := gate.price.OpenDragBudgetPct(symbol); ok && dragBudget != nil {
			if dragBudget.Float64() > 5.0 {
				return GateDecision{Allowed: false, Reason: "excessive entry drag budget (hollow book)"}
			}
		}
	}

	if forceVirtual {
		return GateDecision{Allowed: true, Virtual: true, Reason: "coder uncalibrated; virtual exploration only"}
	}

	brightness := regionBrightness(observed)

	// NET SPARK VECTOR: Detector (R00) + Support (R01) - Opposition (R02)
	netSpark := (brightness[0] + brightness[1]) - brightness[2]

	// NET HOLD VECTOR: Detector (R06) + Support (R07) - Opposition (R08)
	netHold := (brightness[6] + brightness[7]) - brightness[8]

	risk := max(brightness[12], brightness[13], brightness[14]) // R12-R14: Risk Oracle

	// Environmental Test A: Is hold confluence still supportive vs fading?
	lacksIgnition := netHold < gate.ignition

	// Environmental Test B: Is risk dominating the net spark?
	riskDominant := (risk - netSpark) > gate.risk

	if lacksIgnition || riskDominant {
		reason := "environmental caution: "
		if lacksIgnition {
			reason += "low ignition; "
		}
		if riskDominant {
			reason += "risk dominates net spark; "
		}
		return GateDecision{Allowed: true, Virtual: true, Reason: reason}
	}

	return GateDecision{Allowed: true, Virtual: false}
}

func (gate *Gate) EvaluateExit(
	symbol string,
	observed []*nmstore.Region,
	coder CoderReading,
	position *broker.Position,
) GateDecision {
	if position == nil {
		return GateDecision{Allowed: false, Reason: "no open position"}
	}

	inProfit := position.PnL != nil && position.PnL.Sign() > 0

	if !inProfit {
		return GateDecision{Allowed: false, Reason: "trade underwater (downside guarded by stoploss)"}
	}

	brightness := regionBrightness(observed)

	// NET REJECTION VECTOR: Detector (R03) + Support (R04) - Opposition (R05)
	netRejection := (brightness[3] + brightness[4]) - brightness[5]

	// EXHAUSTION: hold opposition (R08) plus any legacy R09 pin still wired
	exhaustion := max(brightness[8], brightness[9])

	// TOTAL EXIT FORCE
	exitForce := math.Max(netRejection, exhaustion)

	// RISK ORACLE: R12 (Friction), R13 (Contagion), R14 (Capacity/Liquidity)
	risk := max(brightness[12], brightness[13], brightness[14])
	capacityRisk := brightness[14]

	// SAFETY VETO: If R14 is screaming, executing a market sell right now will
	// cause excessive slippage and crash the book. Block the cognitive exit and
	// let the StopLoss VWAP-evaporation logic manage the exit safely.
	if capacityRisk > 1.0 && exitForce < capacityRisk {
		return GateDecision{
			Allowed: false,
			Reason:  "exit capacity risk (R14): executing market sell would crash the book",
		}
	}

	// Test A: Has the exit force overcome the learned (bounded) hurdle?
	// (If Macro Contagion R13 is high, we bypass the hurdle and get out)
	if exitForce < gate.rejection && risk < 1.0 {
		return GateDecision{
			Allowed: false,
			Reason:  "exit force below learned hurdle (bids absorbing sells / tape alive)",
		}
	}

	// Test B: Is the predictive coder still projecting strong upward continuation?
	if coder.Bullish && coder.Confidence > 0 && exitForce < (1.0+coder.Confidence) && risk < 1.0 {
		return GateDecision{
			Allowed: false,
			Reason:  "coder forward curve remains bullish (letting runner develop)",
		}
	}

	return GateDecision{Allowed: true, Reason: "exit hurdle cleared"}
}

/*
Update takes one bounded step on every hurdle toward the target the settled
round trip implies, then persists. The return is clamped to [-1,1] before
scaling so a single outsized move cannot run a hurdle out of its band.
*/
func (gate *Gate) Update(result GateResult) {
	r := clamp(result.ReturnFraction, -1.0, 1.0)
	eta := gate.rate

	// ignition: a win lowers the floor (easier), a loss raises it (harder).
	gate.ignition = clamp(gate.ignition-eta*r, gate.ignitionFloor, gate.ignitionCeil)

	// risk: same direction as ignition for entries (a win means tolerate less).
	gate.risk = clamp(gate.risk-eta*r, gate.riskFloor, gate.riskCeil)

	// rejection: a harvested win RAISES the required force (hold longer); a
	// stop-out lowers it (take profits sooner). This is the inverse of the old
	// exp(-return) product, which lowered rejection on wins and clipped them.
	dir := r
	if result.StoppedOut {
		dir = -math.Abs(r)
	}
	gate.rejection = clamp(gate.rejection+eta*dir, gate.rejectionFloor, gate.rejectionCeil)

	gate.persist()
}

// regionBrightness returns the per-region brightness vector (15 entries) from
// the observed regions, indexed by region key.
func regionBrightness(observed []*nmstore.Region) [15]float64 {
	var out [15]float64
	for _, r := range observed {
		if r == nil || int(r.Key) < 0 || int(r.Key) >= 15 {
			continue
		}
		out[r.Key] = r.Brightness
	}
	return out
}
