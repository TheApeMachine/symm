package depthflow

import (
	"fmt"
	"sync"
	"time"

	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/nomagique"
	"github.com/theapemachine/symm/nomagique/calculus"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/equation"
	nmtypes "github.com/theapemachine/symm/nomagique/types"
)

type depthInput struct {
	observedBid calculus.Constant
	observedAsk calculus.Constant
	addBid      calculus.Constant
	addAsk      calculus.Constant
	modifyBid   calculus.Constant
	modifyAsk   calculus.Constant
	deleteBid   calculus.Constant
	deleteAsk   calculus.Constant
	mutationBid calculus.Constant
	mutationAsk calculus.Constant
	elapsed     calculus.Constant
	hasElapsed  calculus.Constant
	hasObserved calculus.Constant
	hasMutation calculus.Constant
}

/*
Level3 is the depth-flow market entity. It maintains an online depth-flow model
per symbol via a single nomagique.Number composition and projects data.Measurement outputs.
*/
type Level3 struct {
	mu     sync.Mutex
	number *nomagique.Pipeline

	in       depthInput
	symbol   string
	at       time.Time
	lastTime map[string]time.Time
}

/*
NewLevel3 constructs the Level3 entity with a single inlined Number composition.
*/
func NewLevel3() *Level3 {
	level3 := &Level3{
		lastTime: make(map[string]time.Time),
	}

	keyFn := func() string { return level3.symbol }
	in := &level3.in

	observedTotal := &nmtypes.Sum{A: &in.observedBid, B: &in.observedAsk}
	observedDiff := &calculus.Difference{Minuend: &in.observedBid, Subtrahend: &in.observedAsk}
	mutationTotal := &nmtypes.Sum{A: &in.mutationBid, B: &in.mutationAsk}
	mutationDiff := &calculus.Difference{Minuend: &in.mutationBid, Subtrahend: &in.mutationAsk}

	mutationImbalance := &calculus.Ratio{Numerator: mutationDiff, Denominator: mutationTotal}
	imbalance := &calculus.Ratio{Numerator: observedDiff, Denominator: observedTotal}
	imbalanceEstimator := &equation.CausalResidual{Key: keyFn}

	rate := &calculus.Ratio{Numerator: observedTotal, Denominator: &in.elapsed}
	rateEstimator := &equation.CausalResidual{Key: keyFn}

	level3.number = nomagique.Number(&nmtypes.Chain{
		A: &nmtypes.Split{
			A: &nmtypes.Report{
				Label: "observed_notional:bid", Unit: "rate", Timescale: "instantaneous",
				Value: &in.observedBid,
			},
			B: &nmtypes.Report{
				Label: "observed_notional:ask", Unit: "rate", Timescale: "instantaneous",
				Value: &in.observedAsk,
			},
			C: &nmtypes.Report{
				Label: "observed_notional", Unit: "rate", Timescale: "instantaneous",
				Value: observedTotal,
			},
			D: &nmtypes.Report{
				Label: "observed_notional_diff", Unit: "rate", Timescale: "instantaneous",
				Value: observedDiff,
			},
		},
		B: &nmtypes.Split{
			A: &nmtypes.Split{
				A: &nmtypes.Report{
					Label: "add_notional:bid", Unit: "rate", Timescale: "instantaneous",
					Value: &in.addBid,
				},
				B: &nmtypes.Report{
					Label: "add_notional:ask", Unit: "rate", Timescale: "instantaneous",
					Value: &in.addAsk,
				},
			},
			B: &nmtypes.Split{
				A: &nmtypes.Report{
					Label: "modify_remaining_notional:bid", Unit: "rate", Timescale: "instantaneous",
					Value: &in.modifyBid,
				},
				B: &nmtypes.Report{
					Label: "modify_remaining_notional:ask", Unit: "rate", Timescale: "instantaneous",
					Value: &in.modifyAsk,
				},
			},
			C: &nmtypes.Split{
				A: &nmtypes.Report{
					Label: "delete_count:bid", Unit: "dimensionless", Timescale: "instantaneous",
					Value: &in.deleteBid,
				},
				B: &nmtypes.Report{
					Label: "delete_count:ask", Unit: "dimensionless", Timescale: "instantaneous",
					Value: &in.deleteAsk,
				},
			},
			D: &nmtypes.Split{
				A: &nmtypes.Report{
					Label: "mutation_count:bid", Unit: "dimensionless", Timescale: "instantaneous",
					Value: &in.mutationBid,
				},
				B: &nmtypes.Report{
					Label: "mutation_count:ask", Unit: "dimensionless", Timescale: "instantaneous",
					Value: &in.mutationAsk,
				},
				C: &nmtypes.Report{
					Label: "mutation_count", Unit: "dimensionless", Timescale: "instantaneous",
					Value: mutationTotal,
				},
				D: &nmtypes.Report{
					Label: "mutation_count_diff", Unit: "dimensionless", Timescale: "instantaneous",
					Value: mutationDiff,
				},
			},
		},
		C: &nmtypes.Split{
			A: &nmtypes.Report{
				Label: "mutation_activity_imbalance", Unit: "dimensionless", Timescale: "instantaneous",
				Value: mutationImbalance, Defined: &in.hasMutation,
			},
			B: &nmtypes.Chain{
				A: &nmtypes.Report{
					Label: "observed_notional_imbalance", Unit: "dimensionless", Timescale: "instantaneous",
					Value: imbalance, Defined: &in.hasObserved,
				},
				B: &nmtypes.Labelled{
					Prefix: "observed_notional_imbalance_",
					Node:   imbalanceEstimator,
				},
			},
			C: &nmtypes.Chain{
				A: &nmtypes.Report{
					Label: "observed_notional_rate", Unit: "per_second", Timescale: "per_second",
					Value: rate, Defined: &in.hasElapsed,
				},
				B: &nmtypes.Labelled{
					Prefix: "observed_notional_rate_",
					Node:   rateEstimator,
				},
			},
		},
		D: &data.Projection{
			Source:   "depthflow",
			Identity: level3.identity,
		},
	})

	return level3
}

/*
Step consumes one Level-3 message once, advances the depth-flow pipeline, and projects a Measurement.
*/
func (level3 *Level3) Step(message kraken.Level3Data) *data.Measurement[float64] {
	if level3 == nil || len(message.Bids)+len(message.Asks) == 0 {
		return nil
	}

	observedBid, addBid, modifyBid, deleteBid, err := observeSide(message.Bids)

	if err != nil {
		return &data.Measurement[float64]{Err: err}
	}

	observedAsk, addAsk, modifyAsk, deleteAsk, err := observeSide(message.Asks)

	if err != nil {
		return &data.Measurement[float64]{Err: err}
	}

	level3.mu.Lock()
	defer level3.mu.Unlock()

	last, hasLast := level3.lastTime[message.Symbol]

	if hasLast && message.Timestamp.Before(last) {
		return nil
	}

	var elapsed float64

	if hasLast {
		elapsed = message.Timestamp.Sub(last).Seconds()

		if elapsed < 0 {
			return nil
		}
	}

	level3.lastTime[message.Symbol] = message.Timestamp
	level3.symbol = message.Symbol
	level3.at = message.Timestamp

	in := &level3.in
	in.observedBid.Value = nmtypes.Number(observedBid)
	in.observedAsk.Value = nmtypes.Number(observedAsk)
	in.addBid.Value = nmtypes.Number(addBid)
	in.addAsk.Value = nmtypes.Number(addAsk)
	in.modifyBid.Value = nmtypes.Number(modifyBid)
	in.modifyAsk.Value = nmtypes.Number(modifyAsk)
	in.deleteBid.Value = nmtypes.Number(deleteBid)
	in.deleteAsk.Value = nmtypes.Number(deleteAsk)
	in.mutationBid.Value = nmtypes.Number(len(message.Bids))
	in.mutationAsk.Value = nmtypes.Number(len(message.Asks))

	in.hasObserved.Value = 0

	if observedBid+observedAsk > 0 {
		in.hasObserved.Value = 1
	}

	in.hasMutation.Value = 0

	if len(message.Bids)+len(message.Asks) > 0 {
		in.hasMutation.Value = 1
	}

	in.elapsed.Value = nmtypes.Number(elapsed)
	in.hasElapsed.Value = 0

	if hasLast && elapsed > 0 {
		in.hasElapsed.Value = 1
	}

	level3.number.Step(1.0)

	return level3.number.Measurement()
}

func (level3 *Level3) identity() (string, string, time.Time, time.Time) {
	return level3.symbol + ":depthflow:" + level3.at.Format(time.RFC3339Nano),
		level3.symbol,
		level3.at,
		level3.at
}

func observeSide(orders []kraken.Level3Order) (observed, added, modified, deleted float64, err error) {
	for _, order := range orders {
		if order.LimitPrice == nil || order.OrderQty == nil {
			return 0, 0, 0, 0, fmt.Errorf("depthflow: level3 order requires price and quantity")
		}

		price := order.LimitPrice.Float64()
		quantity := order.OrderQty.Float64()

		if price <= 0 || quantity < 0 {
			return 0, 0, 0, 0, fmt.Errorf("depthflow: level3 order requires positive price and non-negative quantity")
		}

		notional := price * quantity
		observed += notional

		switch order.Event {
		case "", "add":
			added += notional
		case "modify":
			modified += notional
		case "delete":
			deleted++
		default:
			return 0, 0, 0, 0, fmt.Errorf("depthflow: unknown level3 event %q", order.Event)
		}
	}

	return observed, added, modified, deleted, nil
}

/*
Close releases resources held by the Level3 entity.
*/
func (level3 *Level3) Close() error {
	return nil
}
