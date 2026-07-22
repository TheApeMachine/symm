package strategy

import (
	"maps"
	"sort"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/types"
)

/*
Arbiter ranks enter candidates by utility, fills free slots, and rotates when
a challenger clears the one-step wait gate against an open incumbent.
*/
type Arbiter struct {
	desk    *broker.Desk
	price   *broker.Price
	balance *broker.Balance
	admit   *Admit
	rotate  Rotate
}

/*
NewArbiter wires slot checks, wallet inventory, Admit persistence, and Rotate.
*/
func NewArbiter(
	desk *broker.Desk,
	price *broker.Price,
	balance *broker.Balance,
	admit *Admit,
	rotate Rotate,
) *Arbiter {
	return &Arbiter{
		desk:    desk,
		price:   price,
		balance: balance,
		admit:   admit,
		rotate:  rotate,
	}
}

/*
Select sorts enter candidates by utility, admits into free slots, and otherwise
rotates or rejects.
*/
func (arbiter *Arbiter) Select(thesis *types.Thesis) {
	if err := arbiter.validate(map[string]any{"thesis": thesis}); err != nil {
		return
	}

	enters := make([]types.Decision, 0, len(thesis.Decisions))
	rest := make([]types.Decision, 0, len(thesis.Decisions))

	for _, decision := range thesis.Decisions {
		if decision.Action == types.ActionEnter {
			enters = append(enters, decision)
			continue
		}

		rest = append(rest, decision)
	}

	thesis.Decisions = rest

	sort.Slice(enters, func(left, right int) bool {
		return enters[left].Utility > enters[right].Utility
	})

	incumbents := arbiter.incumbents(thesis)
	open := arbiter.desk.OpenPositions()
	maxNormal := arbiter.desk.MaxSlots(false)
	maxAll := arbiter.desk.MaxSlots(true)
	admittedNormal := 0
	admittedReserved := 0

	for _, decision := range enters {
		opportunity := decision.AllocationClass == "reserved"
		useNormal := open+admittedNormal < maxNormal
		useReserved := opportunity &&
			open+admittedNormal+admittedReserved < maxAll

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

		if arbiter.displace(thesis, &decision, incumbents) {
			continue
		}

		arbiter.admit.Reject(
			thesis, decision, opportunity,
			maxNormal-open, admittedNormal, incumbents,
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
	index, found := arbiter.rotate.Best(decision.Utility, incumbents)

	if !found {
		return false
	}

	incumbent := &incumbents[index]

	if incumbent.Notional == nil || incumbent.Notional.Sign() <= 0 {
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
	decision.Reason = "challenger clears one-step wait threshold against " +
		incumbent.Symbol
	decision.Alternatives["hold_incumbent"] = incumbent.HoldUtility
	decision.Alternatives["exit_cost"] = incumbent.ExitCost
	decision.Alternatives["clear_prob"] = incumbent.ClearProb
	decision.Alternatives["rotate_value"] = rotateValue
	decision.Alternatives["wait_value"] = waitValue
	decision.Alternatives["rotate_surplus"] = arbiter.rotate.Surplus(
		decision.Utility, incumbent.HoldUtility, incumbent.ExitCost,
	)
	decision.DisplacedQuantity = incumbent.Qty.Copy()
	decision.DisplacedPrice = incumbent.Mark.Copy()

	arbiter.admit.Accept(thesis, *decision, opportunity)

	return true
}

/*
incumbents lists open holdings with enough forecast and fee evidence for rotate
comparison.
*/
func (arbiter *Arbiter) incumbents(thesis *types.Thesis) []Incumbent {
	forecasts := selectForecasts(thesis.Forecasts)
	rows := make([]Incumbent, 0)

	for holding := range arbiter.balance.Holdings() {
		if holding.Status != types.OPEN {
			continue
		}

		if arbiter.exiting(thesis, holding.Symbol) {
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

		if holding.Mark == nil || holding.Qty == nil {
			continue
		}

		mark := holding.Mark.Copy()
		quantity := holding.Qty.Copy()
		notional := decimal.ExactMul(mark, quantity)

		if notional == nil || notional.Sign() <= 0 {
			continue
		}

		rows = append(rows, Incumbent{
			Symbol:      holding.Symbol,
			HoldUtility: arbiter.rotate.Hold(forecast),
			ExitCost:    arbiter.rotate.Exit(forecast, fraction.Float64()),
			Notional:    notional,
			Qty:         quantity,
			Mark:        mark,
			ClearProb:   arbiter.rotate.Clear(holding.Stoploss, forecast),
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
