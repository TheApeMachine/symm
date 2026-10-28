package strategy

import (
	"context"
	"math"

	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/data"
	nmstore "github.com/theapemachine/symm/nomagique/store"
	"github.com/theapemachine/symm/store"
)

// GateDecision conveys whether an entry can proceed, and if it must be Virtual.
type GateDecision struct {
	Allowed bool   // false = physical block (cannot trade at all)
	Virtual bool   // true = sandbox only, false = permitted for Paper
	Reason  string // diagnostic reason
}

type Gate struct {
	ctx   context.Context
	desk  *broker.Desk
	store *store.Client

	// Learned environmental hurdles (compound via e^-return)
	ignitionHurdle float64 // Minimum ignition required for Paper
	riskTolerance  float64 // Maximum risk-over-spark tolerated for Paper
}

func NewGate(ctx context.Context, desk *broker.Desk, client *store.Client) *Gate {
	gate := &Gate{
		ctx:            ctx,
		desk:           desk,
		store:          client,
		ignitionHurdle: 1.0, // Base Unit tension
		riskTolerance:  1.0, // Base Unit tension
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
		}
	}

	return gate
}

/*
Evaluate inspects physical invariants and environmental posture.
Physical impossibilities block entirely; environmental doubts relegate to Virtual.
*/
func (gate *Gate) Evaluate(
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

	brightness := func(key uint8) float64 {
		for _, r := range observed {
			if r != nil && r.Key == key {
				return r.Brightness
			}
		}

		return 0.0
	}

	spark := brightness(0)                                      // R00: The Catalyst
	ignition := brightness(7)                                   // R07: Explosive Ignition
	risk := max(brightness(12), brightness(13), brightness(14)) // Friction, Contagion, Capacity

	// Environmental Test A: Has ignition cleared the learned hurdle?
	lacksIgnition := ignition < gate.ignitionHurdle

	// Environmental Test B: Is risk dominating spark beyond our learned tolerance?
	riskDominant := (risk - spark) > gate.riskTolerance

	// If environmental posture is questionable, DO NOT discard the trade:
	// Relegate it to Virtual space so it can test the environment with 0 capital risk!
	if lacksIgnition || riskDominant {
		reason := "environmental caution: "

		if lacksIgnition {
			reason += "low ignition; "
		}

		if riskDominant {
			reason += "risk dominates spark; "
		}

		return GateDecision{Allowed: true, Virtual: true, Reason: reason}
	}

	// All hurdles cleared: permitted for real Paper capital
	return GateDecision{Allowed: true, Virtual: false}
}

/*
Update compounds the gate's hurdles using the realized market return:
Hurdle * exp(-Return). Both Paper and Virtual trades train the Gate.
*/
func (gate *Gate) Update(returnFraction float64) {
	decay := math.Exp(-returnFraction)

	// Winning trades relax hurdles; losing trades tighten them
	gate.ignitionHurdle *= decay
	gate.riskTolerance *= decay

	// Persist updated hurdles
	if gate.store != nil {
		value := data.NewValue("model", "parameters", "gate")

		value.Set(gate.ignitionHurdle, "ignition_hurdle")
		value.Set(gate.riskTolerance, "risk_tolerance")

		gate.store.WriteAtPrefix("model/parameters/gate.bin", value.Bytes())
	}
}
