package strategy

import (
	"math"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/theapemachine/symm/types"
)

/*
Incumbent is one open holding's continuation value for rotate comparisons.
HoldUtility is the forward SNR of staying; ExitCost is the one-way friction
paid to free the slot; Notional sizes the challenger from capital being freed.
*/
type Incumbent struct {
	Symbol      string
	HoldUtility float64
	ExitCost    float64
	Notional    *decimal.Decimal
	Qty         *decimal.Decimal
	Mark        *decimal.Decimal
	ClearProb   float64
	Displaced   bool
}

/*
Rotate owns one-step rotate arithmetic: keep score, exit friction, surplus,
weakest incumbent, and the Bellman gate.
*/
type Rotate struct{}

/*
NewRotate returns the rotate arithmetic shell.
*/
func NewRotate() Rotate {
	return Rotate{}
}

/*
Hold is the executable value of keeping an open name: expected return net of
uncertainty. Entry fees are sunk and do not belong in the keep score.
*/
func (rotate Rotate) Hold(forecast types.Forecasts) float64 {
	return forecast.ExpectedReturn - forecast.Uncertainty
}

/*
Exit is the full one-way friction paid to liquidate an incumbent: fee,
half-spread, impact, and adverse selection.
*/
func (rotate Rotate) Exit(forecast types.Forecasts, fee float64) float64 {
	return fee +
		forecast.ExpectedSpread/2 +
		forecast.ExpectedImpact +
		forecast.ExpectedAdverseSelection
}

/*
Surplus is the net utility of replacing an incumbent with a challenger.
Positive means rotate; non-positive means wait for the open thesis to mature.
*/
func (rotate Rotate) Surplus(challenger, hold, exitCost float64) float64 {
	return challenger - hold - exitCost
}

/*
Weakest returns the displaceable incumbent with the lowest hold utility.
*/
func (rotate Rotate) Weakest(incumbents []Incumbent) (int, bool) {
	best := -1

	for index := range incumbents {
		if incumbents[index].Displaced {
			continue
		}

		weakness := incumbents[index].HoldUtility + incumbents[index].ExitCost

		if best < 0 ||
			weakness < incumbents[best].HoldUtility+incumbents[best].ExitCost {
			best = index
		}
	}

	return best, best >= 0
}

/*
Gate reports whether enter beats keep after exit friction once the option
value of a native clear is charged against the edge.
*/
func (rotate Rotate) Gate(enter, keep, exitCost, clearProb float64) bool {
	edge := enter - keep

	if edge <= 0 {
		return false
	}

	return edge*(1-math.Min(1, math.Max(0, clearProb))) > exitCost
}

/*
Advantage is the one-step surplus of displacing an incumbent.
*/
func (rotate Rotate) Advantage(enter, keep, exitCost, clearProb float64) float64 {
	return (enter-keep)*(1-math.Min(1, math.Max(0, clearProb))) - exitCost
}

/*
Best picks the eligible incumbent with the largest positive advantage.
*/
func (rotate Rotate) Best(enter float64, incumbents []Incumbent) (int, bool) {
	best := -1
	bestAdvantage := 0.0

	for index := range incumbents {
		if incumbents[index].Displaced {
			continue
		}

		advantage := rotate.Advantage(
			enter,
			incumbents[index].HoldUtility,
			incumbents[index].ExitCost,
			incumbents[index].ClearProb,
		)

		if advantage <= 0 {
			continue
		}

		if best < 0 || advantage > bestAdvantage {
			best = index
			bestAdvantage = advantage
		}
	}

	return best, best >= 0
}

/*
Clear is the calibrated one-horizon P(native clear): stop or take-profit frees
the slot without a forced rotate.
*/
func (rotate Rotate) Clear(stop *types.Stoploss, forecast types.Forecasts) float64 {
	if stop == nil {
		return 0
	}

	stop.RLock()
	defer stop.RUnlock()

	if stop.Action == "stop" || stop.Action == "take_profit" {
		return 1
	}

	if stop.PeakReturn <= 0 || stop.TrailDistance <= 0 {
		return 0
	}

	if forecast.ExpectedReturn > 0 {
		return 0
	}

	if !forecast.Calibrated || forecast.Confidence <= 0 {
		return 0
	}

	giveback := math.Max(0, stop.PeakReturn-stop.MarkReturn)
	proximity := math.Min(1, 1-giveback/stop.TrailDistance)

	if proximity <= 0 {
		return 0
	}

	confidence := math.Min(1, forecast.Confidence)
	skill := 1.0

	if forecast.Uncertainty > 0 && forecast.IncrementalMSE > 0 {
		rmse := math.Sqrt(forecast.IncrementalMSE)
		// Lower residual error raises clear score; RMSE in the numerator was inverted.
		skill = forecast.Uncertainty / (rmse + forecast.Uncertainty)
	}

	return math.Min(1, proximity*confidence*skill)
}

/*
Commit materializes rotation exits for sized enter decisions and drops unsized enters.
*/
func (rotate Rotate) Commit(thesis *types.Thesis) {
	if thesis == nil {
		return
	}

	exits := make([]types.Decision, 0)

	for index := range thesis.Decisions {
		decision := &thesis.Decisions[index]

		if decision.Action == types.ActionEnter && decision.ProposedQuantity == nil {
			decision.Action = types.ActionNothing
			decision.Displaces = ""
			decision.ProposedQuantity = nil
			decision.ProposedNotional = nil
			thesis.Holdings.Delete(decision.Symbol)
			thesis.NoteLifecycle(
				decision.Symbol, types.LifecycleRejected, decision.At,
			)

			if decision.Reason == "" {
				decision.Reason = "unsized"
			}

			continue
		}

		if decision.Action != types.ActionEnter ||
			decision.Cause != "rotation" ||
			decision.Displaces == "" ||
			decision.ProposedQuantity == nil {
			continue
		}

		if decision.DisplacedQuantity == nil || decision.DisplacedPrice == nil {
			decision.Action = types.ActionNothing
			decision.Reason = "rotation source money is unavailable"
			thesis.Holdings.Delete(decision.Symbol)
			thesis.NoteLifecycle(
				decision.Symbol, types.LifecycleRejected, decision.At,
			)
			continue
		}

		phase, found := thesis.Lifecycle.Load(decision.Displaces)

		if found && (phase == types.LifecycleExitSelected ||
			phase == types.LifecycleExitSubmitted ||
			phase == types.LifecycleClosed) {
			continue
		}

		exits = append(exits, types.Decision{
			Action:  types.ActionExit,
			Symbol:  decision.Displaces,
			At:      decision.At,
			Utility: -decision.Alternatives["exit_cost"],
			Alternatives: map[string]float64{
				"exit": -decision.Alternatives["exit_cost"],
				"hold": decision.Alternatives["hold_incumbent"],
			},
			ProposedQuantity:  decision.DisplacedQuantity.Copy(),
			ReferencePrice:    decision.DisplacedPrice.Copy(),
			ValidThroughEpoch: decision.ValidThroughEpoch,
			Cause:             "rotation",
			Reason:            "displaced by higher-utility challenger " + decision.Symbol,
		})

		thesis.NoteLifecycle(decision.Displaces, types.LifecycleExitSelected, decision.At)
	}

	thesis.Decisions = append(thesis.Decisions, exits...)
}
