package strategy

import (
	"sort"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/types"
)

/*
Arbiter selects portfolio actions under slot constraints by one-step
lookahead. Free slots fill by local enter utility; when full, rotate only if
net edge beats the option value of waiting for a native clear.
*/
type Arbiter struct {
	desk    *broker.Desk
	price   *broker.Price
	planner *Planner
}

/*
NewArbiter wires Desk slot arithmetic and Price fee fractions for Select.
*/
func NewArbiter(desk *broker.Desk, price *broker.Price) *Arbiter {
	return &Arbiter{desk: desk, price: price}
}

/*
Select peels Measure enter candidates, admits into free slots, and otherwise
rotates or waits using the Bellman one-step gate.
*/
func (arbiter *Arbiter) Select(thesis *types.Thesis) {
	if err := errnie.Error(errnie.Require(map[string]any{
		"arbiter": arbiter,
		"desk":    arbiter.desk,
		"price":   arbiter.price,
		"planner": arbiter.planner,
		"thesis":  thesis,
	})); err != nil {
		return
	}

	candidates, rest := peelEnters(thesis.Decisions)
	thesis.Decisions = rest

	sort.Slice(candidates, func(left, right int) bool {
		return dollarUtility(candidates[left]) > dollarUtility(candidates[right])
	})

	rotatable := arbiter.rotatable(thesis)
	open := arbiter.desk.OpenPositions()
	freeNormal, freeReserved := arbiter.desk.Free(open)
	admittedNormal := 0
	admittedReserved := 0

	for _, decision := range candidates {
		if arbiter.desk.Pending(decision.Symbol) {
			arbiter.planner.persistRejectedEntry(
				thesis, decision, decision.AllocationClass == "reserved",
				freeNormal, admittedNormal, rotatable,
			)
			continue
		}

		opportunity := decision.AllocationClass == "reserved"
		useNormal := admittedNormal < freeNormal
		useReserved := opportunity && admittedReserved < freeReserved

		if useNormal || useReserved {
			if useNormal {
				admittedNormal++
			}

			if !useNormal && useReserved {
				admittedReserved++
			}

			arbiter.planner.persistAcceptedEntry(thesis, decision, opportunity)

			continue
		}

		if arbiter.displace(thesis, &decision, rotatable) {
			continue
		}

		arbiter.planner.persistRejectedEntry(
			thesis, decision, opportunity, freeNormal, admittedNormal, rotatable,
		)
	}
}

/*
displace replaces the weakest open holding when the challenger's enter utility
clears the one-step wait threshold against that incumbent.
*/
func (arbiter *Arbiter) displace(
	thesis *types.Thesis,
	decision *types.Decision,
	incumbents []Incumbent,
) bool {
	opportunity := decision.AllocationClass == "reserved"

	if !arbiter.desk.HasSlotAfter(opportunity, 1) {
		return false
	}

	index, found := bestRotation(decision.Utility, incumbents)

	if !found {
		return false
	}

	incumbent := &incumbents[index]

	if incumbent.Notional <= 0 {
		return false
	}

	arbiter.planner.scaleTo(decision, incumbent.Notional)
	incumbent.Displaced = true

	edge := decision.Utility - incumbent.HoldUtility
	rotateValue := edge - incumbent.ExitCost
	waitValue := edge * incumbent.ClearScore

	if decision.Alternatives == nil {
		decision.Alternatives = map[string]float64{}
	}

	decision.Cause = "rotation"
	decision.Displaces = incumbent.Symbol
	decision.Reason = "challenger clears one-step wait threshold against " + incumbent.Symbol
	decision.Alternatives["hold_incumbent"] = incumbent.HoldUtility
	decision.Alternatives["exit_cost"] = incumbent.ExitCost
	decision.Alternatives["clear_score"] = incumbent.ClearScore
	decision.Alternatives["rotate_value"] = rotateValue
	decision.Alternatives["wait_value"] = waitValue
	decision.Alternatives["rotate_surplus"] = rotateSurplus(
		decision.Utility, incumbent.HoldUtility, incumbent.ExitCost,
	)
	decision.Alternatives["incumbent_qty"] = incumbent.Qty
	decision.Alternatives["incumbent_mark"] = incumbent.Mark

	arbiter.planner.persistAcceptedEntry(
		thesis, *decision, opportunity,
	)

	return true
}

/*
dollarUtility ranks enters by return-space utility times feasible notional so
slot admission prefers capital that can actually clear.
*/
func dollarUtility(decision types.Decision) float64 {
	notional := 0.0

	if decision.ProposedNotional != nil {
		notional = decision.ProposedNotional.Float64()
	}

	if notional <= 0 && decision.AvailableCapital != nil {
		notional = decision.AvailableCapital.Float64()
	}

	if notional <= 0 {
		return decision.Utility
	}

	return decision.Utility * notional
}

/*
rotatable projects open holdings that have fee and forecast evidence complete
enough for rotate comparison. Slot capacity itself uses Desk.OpenPositions.
*/
func (arbiter *Arbiter) rotatable(thesis *types.Thesis) []Incumbent {
	forecasts := map[string]types.Forecasts{}

	for _, forecast := range thesis.Forecasts {
		forecasts[forecast.Symbol] = forecast
	}

	rows := make([]Incumbent, 0)

	thesis.Holdings.Range(func(_, value any) bool {
		holding, ok := value.(*types.Holding)

		if !ok || holding == nil || holding.Status == types.CLOSED {
			return true
		}

		if exiting(thesis, holding.Symbol) {
			return true
		}

		if arbiter.desk.Pending(holding.Symbol) {
			return true
		}

		forecast, found := forecasts[holding.Symbol]

		if !found {
			return true
		}

		fraction, err := arbiter.price.Fraction(holding.Symbol)

		if err != nil {
			return true
		}

		notional := 0.0
		qty := 0.0
		mark := 0.0

		if holding.Mark != nil && holding.Qty != nil {
			mark = holding.Mark.Float64()
			qty = holding.Qty.Float64()
			notional = mark * qty
		}

		rows = append(rows, Incumbent{
			Symbol:      holding.Symbol,
			HoldUtility: arbiter.planner.holdUtility(forecast),
			ExitCost:    arbiter.planner.exitCost(forecast, fraction.Float64()),
			Notional:    notional,
			Qty:         qty,
			Mark:        mark,
			ClearScore:  clearScore(holding.Stoploss, forecast),
		})

		return true
	})

	return rows
}

/*
clearScore is the one-horizon mass that an incumbent frees its slot natively
(stop/take-profit) without a forced rotate exit. Not a calibrated probability.
*/
func clearScore(stop *types.Stoploss, forecast types.Forecasts) float64 {
	if stop == nil {
		return 0
	}

	if stop.Action == "stop" || stop.Action == "take_profit" {
		return 1
	}

	if stop.PeakReturn <= 0 || stop.TrailDistance <= 0 {
		return 0
	}

	if forecast.ExpectedReturn > 0 {
		return 0
	}

	giveback := stop.PeakReturn - stop.MarkReturn

	if giveback < 0 {
		giveback = 0
	}

	proximity := 1 - giveback/stop.TrailDistance

	if proximity <= 0 {
		return 0
	}

	if proximity > 1 {
		return 1
	}

	return proximity
}

/*
shouldRotate reports whether enter beats keep after exit friction once the
option value of a native clear is charged against the edge.
*/
func shouldRotate(enter, keep, exitCost, clearScore float64) bool {
	edge := enter - keep

	if edge <= 0 {
		return false
	}

	if clearScore < 0 {
		clearScore = 0
	}

	if clearScore > 1 {
		clearScore = 1
	}

	return edge*(1-clearScore) > exitCost
}

/*
rotationAdvantage is the one-step surplus of displacing an incumbent.
*/
func rotationAdvantage(enter, keep, exitCost, clearScore float64) float64 {
	edge := enter - keep

	if clearScore < 0 {
		clearScore = 0
	}

	if clearScore > 1 {
		clearScore = 1
	}

	return edge*(1-clearScore) - exitCost
}

/*
bestRotation picks the eligible incumbent with the largest positive advantage.
*/
func bestRotation(enter float64, incumbents []Incumbent) (int, bool) {
	best := -1
	bestAdvantage := 0.0

	for index := range incumbents {
		if incumbents[index].Displaced {
			continue
		}

		advantage := rotationAdvantage(
			enter,
			incumbents[index].HoldUtility,
			incumbents[index].ExitCost,
			incumbents[index].ClearScore,
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

func peelEnters(decisions []types.Decision) (enters, rest []types.Decision) {
	enters = make([]types.Decision, 0, len(decisions))
	rest = make([]types.Decision, 0, len(decisions))

	for _, decision := range decisions {
		if decision.Action == types.ActionEnter {
			enters = append(enters, decision)

			continue
		}

		rest = append(rest, decision)
	}

	return enters, rest
}
