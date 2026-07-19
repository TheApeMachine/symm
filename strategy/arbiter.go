package strategy

import (
	"maps"
	"sort"

	"github.com/spf13/viper"
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
	desk        *broker.Desk
	price       *broker.Price
	balance     *broker.Balance
	admit       *Admit
	rotate      Rotate
	maxFraction float64
}

/*
NewArbiter wires Desk slot arithmetic, wallet qty, Admit persistence, and
Rotate gates.
*/
func NewArbiter(
	desk *broker.Desk,
	price *broker.Price,
	balance *broker.Balance,
	admit *Admit,
	rotate Rotate,
) *Arbiter {
	return &Arbiter{
		desk:        desk,
		price:       price,
		balance:     balance,
		admit:       admit,
		rotate:      rotate,
		maxFraction: viper.GetFloat64("trading.allocation.max_fraction"),
	}
}

/*
Select peels Measure enter candidates, admits into free slots, and otherwise
rotates or waits using the Bellman one-step gate.
*/
func (arbiter *Arbiter) Select(thesis *types.Thesis) {
	if err := arbiter.validate(map[string]any{"thesis": thesis}); err != nil {
		return
	}

	candidates, rest := arbiter.peel(thesis.Decisions)
	thesis.Decisions = rest

	cash := arbiter.cash()
	forecasts := selectForecasts(thesis.Forecasts)

	sort.Slice(candidates, func(left, right int) bool {
		return arbiter.dollar(candidates[left], cash, forecasts) >
			arbiter.dollar(candidates[right], cash, forecasts)
	})

	rotatable := arbiter.rotatable(thesis)
	open := arbiter.desk.OpenPositions()
	freeNormal, freeReserved := arbiter.desk.Free(open)
	admittedNormal := 0
	admittedReserved := 0

	for _, decision := range candidates {
		if arbiter.desk.Pending(decision.Symbol) {
			arbiter.admit.Reject(
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

			arbiter.admit.Accept(thesis, decision, opportunity)

			continue
		}

		if arbiter.displace(thesis, &decision, rotatable) {
			continue
		}

		arbiter.admit.Reject(
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

	index, found := arbiter.rotate.Best(decision.Utility, incumbents)

	if !found {
		return false
	}

	incumbent := &incumbents[index]

	if incumbent.Notional <= 0 {
		return false
	}

	arbiter.admit.Scale(decision, incumbent.Notional)
	incumbent.Displaced = true

	edge := decision.Utility - incumbent.HoldUtility
	rotateValue := edge - incumbent.ExitCost
	waitValue := edge * incumbent.ClearProb

	if decision.Alternatives == nil {
		decision.Alternatives = map[string]float64{}
	}

	decision.Cause = "rotation"
	decision.Displaces = incumbent.Symbol
	decision.Reason = "challenger clears one-step wait threshold against " + incumbent.Symbol
	decision.Alternatives["hold_incumbent"] = incumbent.HoldUtility
	decision.Alternatives["exit_cost"] = incumbent.ExitCost
	decision.Alternatives["clear_prob"] = incumbent.ClearProb
	decision.Alternatives["rotate_value"] = rotateValue
	decision.Alternatives["wait_value"] = waitValue
	decision.Alternatives["rotate_surplus"] = arbiter.rotate.Surplus(
		decision.Utility, incumbent.HoldUtility, incumbent.ExitCost,
	)
	decision.Alternatives["incumbent_qty"] = incumbent.Qty
	decision.Alternatives["incumbent_mark"] = incumbent.Mark

	arbiter.admit.Accept(thesis, *decision, opportunity)

	return true
}

/*
cash reports unreserved quote capital for prequoting fresh-enter notionals.
Missing wallet state ranks by bare utility rather than inventing a dollar value.
*/
func (arbiter *Arbiter) cash() float64 {
	available, err := arbiter.balance.AvailableCash()

	if err != nil || available == nil {
		return 0
	}

	return available.Float64()
}

/*
dollar ranks a candidate by dollar-utility so a lower per-unit edge on a large
deployable position can outrank a sharp edge on a thin one. Rotation challengers
already carry a stamped ProposedNotional; fresh enters have none at Select time,
so their notional is prequoted from unreserved cash and the symbol's visible buy
capacity. When wallet cash is unavailable the whole set ranks by bare utility —
one consistent scale, never bare utility mixed with dollar products in one sort.
*/
func (arbiter *Arbiter) dollar(
	decision types.Decision,
	cash float64,
	forecasts map[string]types.Forecasts,
) float64 {
	if cash <= 0 {
		return decision.Utility
	}

	if decision.ProposedNotional != nil && decision.ProposedNotional.Sign() > 0 {
		return decision.Utility * decision.ProposedNotional.Float64()
	}

	return decision.Utility * arbiter.feasible(cash, forecasts[decision.Symbol])
}

/*
feasible prequotes the deployable notional for a fresh enter as max_fraction of
unreserved cash, capped by the symbol's visible buy capacity so a thin book
cannot inflate a candidate's rank beyond the depth it can absorb.
*/
func (arbiter *Arbiter) feasible(cash float64, forecast types.Forecasts) float64 {
	notional := cash * arbiter.maxFraction

	if forecast.BuyCapacity > 0 && forecast.BuyCapacity < notional {
		return forecast.BuyCapacity
	}

	return notional
}

/*
rotatable projects open Balance lots that have fee and forecast evidence
complete enough for rotate comparison. Slot capacity uses Desk.OpenPositions.
*/
func (arbiter *Arbiter) rotatable(thesis *types.Thesis) []Incumbent {
	forecasts := selectForecasts(thesis.Forecasts)
	rows := make([]Incumbent, 0)

	for holding := range arbiter.balance.Holdings() {
		if holding.Status != types.OPEN {
			continue
		}

		if arbiter.exiting(thesis, holding.Symbol) {
			continue
		}

		if arbiter.desk.Pending(holding.Symbol) {
			continue
		}

		forecast, found := forecasts[holding.Symbol]

		if !found {
			continue
		}

		fraction, err := arbiter.price.Fraction(holding.Symbol)

		if err != nil {
			thesis.Decisions = append(thesis.Decisions, types.Decision{
				Action: types.ActionHold,
				Symbol: holding.Symbol,
				Cause:  "rotation",
				Reason: "fee schedule unavailable; keep incumbent",
			})

			continue
		}

		notional := 0.0
		qty := 0.0
		mark := 0.0

		if holding.Mark != nil && holding.Qty != nil {
			mark = holding.Mark.Float64()
			qty = holding.Qty.Float64()
			notional = mark * qty
		}

		var stop *types.Stoploss

		if position, ok := arbiter.desk.Position(holding.Symbol); ok {
			stop = position.Stop()
		}

		rows = append(rows, Incumbent{
			Symbol:      holding.Symbol,
			HoldUtility: arbiter.rotate.Hold(forecast),
			ExitCost:    arbiter.rotate.Exit(forecast, fraction.Float64()),
			Notional:    notional,
			Qty:         qty,
			Mark:        mark,
			ClearProb:   arbiter.rotate.Clear(stop, forecast),
		})
	}

	return rows
}

func (arbiter *Arbiter) exiting(thesis *types.Thesis, symbol string) bool {
	phase, found := thesis.Lifecycle.Load(symbol)

	return found &&
		(phase == types.LifecycleExitSelected ||
			phase == types.LifecycleExitSubmitted)
}

func (arbiter *Arbiter) peel(decisions []types.Decision) (enters, rest []types.Decision) {
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

func (arbiter *Arbiter) validate(mandatory map[string]any) error {
	check := map[string]any{
		"desk":    arbiter.desk,
		"price":   arbiter.price,
		"balance": arbiter.balance,
		"admit":   arbiter.admit,
	}

	maps.Copy(check, mandatory)

	return errnie.Error(errnie.Require(check))
}
