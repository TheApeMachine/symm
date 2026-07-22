package signal

import (
	"fmt"
	"slices"
	"time"
)

/*
ActionKind identifies one explicit economic cause applied to the simulated venue.
*/
type ActionKind uint8

const (
	MoveMid ActionKind = iota
	Trade
	Add
	Cancel
	Refill
	WidenSpread
	TightenSpread
)

/*
Action moves one symbol's book or executes one aggressor trade at its touch.
*/
type Action struct {
	Kind    ActionKind
	Symbol  string
	Side    string
	Ticks   int64
	Qty     float64
	Price   float64
	OrderID string
}

/*
Step groups simultaneous market actions after one deterministic clock advance.
*/
type Step struct {
	Advance time.Duration
	Actions []Action
}

/*
Apply advances the venue by one step and exposes one coherent sample per symbol.
*/
func (signal *Signal) Apply(step Step) error {
	draft := signal.clone()

	if err := draft.apply(step); err != nil {
		return err
	}

	*signal = *draft
	return nil
}

/*
apply mutates only a private draft whose complete result is committed by Apply.
*/
func (signal *Signal) apply(step Step) error {
	if step.Advance <= 0 {
		return fmt.Errorf("signal: positive step advance required")
	}

	signal.at = signal.at.Add(step.Advance)
	before := signal.clone()
	fills := make(map[string][]Fill, len(signal.symbols))

	for _, action := range step.Actions {
		market, exists := signal.markets[action.Symbol]

		if !exists {
			return fmt.Errorf("signal: unknown symbol %q", action.Symbol)
		}

		switch action.Kind {
		case MoveMid:
			market.shift(
				action.Symbol, &signal.nextID, signal.at,
				action.Ticks, action.Ticks,
			)
		case Trade:
			if !onQuantityGrid(action.Qty) ||
				action.Side != "buy" && action.Side != "sell" {
				return fmt.Errorf("signal: valid trade side and quantity required")
			}

			matched, err := market.execute(
				action.Side, action.Qty, signal.at, &signal.nextTrade,
			)

			if err != nil {
				return err
			}

			fills[action.Symbol] = append(fills[action.Symbol], matched...)
		case Add:
			if action.Price <= 0 || !onQuantityGrid(action.Qty) || !validSide(action.Side) ||
				!onTick(action.Price) {
				return fmt.Errorf("signal: valid add side, price, and quantity required")
			}

			market.add(
				action.Symbol, action.Side, action.Price, action.Qty,
				&signal.nextID, signal.at,
			)
		case Cancel:
			if action.OrderID == "" || !market.cancel(action.OrderID) {
				return fmt.Errorf("signal: resting order %q not found", action.OrderID)
			}
		case Refill:
			if !onQuantityGrid(action.Qty) || !validSide(action.Side) {
				return fmt.Errorf("signal: valid refill side and quantity required")
			}

			if err := market.refill(action.Side, action.Qty, signal.at); err != nil {
				return err
			}
		case WidenSpread:
			if action.Ticks <= 0 {
				return fmt.Errorf("signal: positive spread widening required")
			}

			market.shift(
				action.Symbol, &signal.nextID, signal.at,
				-action.Ticks, action.Ticks,
			)
		case TightenSpread:
			if action.Ticks <= 0 {
				return fmt.Errorf("signal: positive spread tightening required")
			}

			market.shift(
				action.Symbol, &signal.nextID, signal.at,
				action.Ticks, -action.Ticks,
			)
		default:
			return fmt.Errorf("signal: unknown action kind %d", action.Kind)
		}
	}

	for _, symbol := range signal.symbols {
		if err := signal.markets[symbol].validate(); err != nil {
			return fmt.Errorf("signal: invalid %s book: %w", symbol, err)
		}
	}

	samples := make([]Sample, len(signal.symbols))

	for index, symbol := range signal.symbols {
		bookChanged := !sameOrders(before.markets[symbol], signal.markets[symbol])
		beforeQuote, beforeExists := before.markets[symbol].quote()
		afterQuote, afterExists := signal.markets[symbol].quote()
		touchChanged := beforeExists != afterExists || beforeQuote != afterQuote
		samples[index] = signal.markets[symbol].sample(
			symbol, signal.at, fills[symbol], bookChanged, touchChanged,
		)
	}

	signal.tape = [][]Sample{samples}
	signal.phase++
	return nil
}

/*
sameOrders compares the complete authoritative resting state for sparse output.
*/
func sameOrders(left *symbolState, right *symbolState) bool {
	return slices.Equal(left.bids, right.bids) && slices.Equal(left.asks, right.asks)
}
