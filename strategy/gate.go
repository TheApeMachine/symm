package strategy

import (
	"context"
	"math"

	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/data"
	nmstore "github.com/theapemachine/symm/nomagique/store"
	"github.com/theapemachine/symm/store"
)

// GateDecision conveys whether an action can proceed, and if it must be Virtual.
type GateDecision struct {
	Allowed bool   // false = blocked by gate
	Virtual bool   // true = sandbox only, false = permitted for Paper
	Reason  string // diagnostic reason
}

type Gate struct {
	ctx   context.Context
	desk  *broker.Desk
	price *broker.Price
	store *store.Client

	// Learned environmental hurdles for ENTERS (compound via e^-return)
	ignitionHurdle float64 // Minimum ignition required for Paper
	riskTolerance  float64 // Maximum risk-over-spark tolerated for Paper

	// Learned environmental hurdles for EXITS (compound via e^-return)
	rejectionHurdle float64 // Minimum rejection/evaporation force required to harvest
}

func NewGate(
	ctx context.Context,
	desk *broker.Desk,
	price *broker.Price,
	client *store.Client,
) *Gate {
	gate := &Gate{
		ctx:             ctx,
		desk:            desk,
		price:           price,
		store:           client,
		ignitionHurdle:  1.0, // Base Unit tension
		riskTolerance:   1.0, // Base Unit tension
		rejectionHurdle: 1.0, // Base Unit tension
	}

	// Restore gate state from S3 if available
	if client != nil {
		if val := client.Download(ctx, "model/parameters/gate.bin"); val != nil && val.Err == nil {
			if h := val.GetFloat64("ignition_hurdle"); h > 0 {
				gate.ignitionHurdle = h
			}
			if r := val.GetFloat64("risk_tolerance"); r > 0 {
				gate.riskTolerance = r
			}
			if rej := val.GetFloat64("rejection_hurdle"); rej > 0 {
				gate.rejectionHurdle = rej
			}
		}
	}

	return gate
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
	if !coder.Bullish {
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

	brightness := func(key uint8) float64 {
		for _, r := range observed {
			if r != nil && r.Key == key {
				return r.Brightness
			}
		}
		return 0.0
	}

	// NET SPARK VECTOR: Detector (R00) + Support (R01) - Opposition (R02)
	netSpark := (brightness(0) + brightness(1)) - brightness(2)

	ignition := brightness(7) // R07: Explosive Ignition
	risk := max(brightness(12), brightness(13), brightness(14)) // R12-R14: Risk Oracle

	// Environmental Test A: Has ignition cleared the learned hurdle?
	lacksIgnition := ignition < gate.ignitionHurdle

	// Environmental Test B: Is risk dominating the net spark?
	riskDominant := (risk - netSpark) > gate.riskTolerance

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

	brightness := func(key uint8) float64 {
		for _, r := range observed {
			if r != nil && r.Key == key {
				return r.Brightness
			}
		}
		return 0.0
	}

	// NET REJECTION VECTOR: Detector (R03) + Support (R04) - Opposition (R05)
	netRejection := (brightness(3) + brightness(4)) - brightness(5)
	
	// EXHAUSTION: Turbulent Friction (R08) / Dead Starved Tape (R09)
	exhaustion := max(brightness(8), brightness(9))
	
	// TOTAL EXIT FORCE
	exitForce := math.Max(netRejection, exhaustion)

	// RISK ORACLE: R12 (Friction), R13 (Contagion), R14 (Capacity/Liquidity)
	risk := max(brightness(12), brightness(13), brightness(14))
	capacityRisk := brightness(14)

	// SAFETY VETO: If R14 is screaming, executing a market sell right now will cause
	// excessive slippage and crash the book. We block the cognitive exit and let the 
	// StopLoss VWAP-evaporation logic manage the exit safely.
	if capacityRisk > 1.0 && exitForce < capacityRisk {
		return GateDecision{
			Allowed: false,
			Reason:  "exit capacity risk (R14): executing market sell would crash the book",
		}
	}

	// Test A: Has the exit force overcome the learned hurdle?
	// (If Macro Contagion R13 is high, we bypass the hurdle and get out)
	if exitForce < gate.rejectionHurdle && risk < 1.0 {
		return GateDecision{
			Allowed: false,
			Reason:  "exit force below learned hurdle (bids absorbing sells / tape alive)",
		}
	}

	// Test B: Is the predictive coder still projecting strong upward continuation?
	if coder.Bullish && exitForce < (1.0+coder.Confidence) && risk < 1.0 {
		return GateDecision{
			Allowed: false,
			Reason:  "coder forward curve remains bullish (letting runner develop)",
		}
	}

	return GateDecision{Allowed: true, Reason: "exit hurdle cleared"}
}

/*
Update compounds both enter and exit hurdles using the realized market return:
Hurdle * exp(-Return). Both Paper and Virtual trades train the Gate.
*/
func (gate *Gate) Update(returnFraction float64) {
	decay := math.Exp(-returnFraction)

	// Enters: winning trades relax hurdles; losing trades tighten them
	gate.ignitionHurdle *= decay
	gate.riskTolerance *= decay

	// Exits: healthy profit harvests relax the rejection hurdle;
	// premature or clipped exits tighten it (demanding stronger exhaustion before exiting).
	gate.rejectionHurdle *= decay

	// Persist updated hurdles
	if gate.store != nil {
		value := data.NewValue("model", "parameters", "gate")
		value.Set(gate.ignitionHurdle, "ignition_hurdle")
		value.Set(gate.riskTolerance, "risk_tolerance")
		value.Set(gate.rejectionHurdle, "rejection_hurdle")
		gate.store.WriteAtPrefix("model/parameters/gate.bin", value.Bytes())
	}
}
