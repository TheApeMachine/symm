package signal

import (
	"fmt"
	"math"
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
	if step.Advance <= 0 {
		return fmt.Errorf("signal: positive step advance required")
	}

	signal.at = signal.at.Add(step.Advance)
	trades := make(map[string]Action, len(signal.symbols))

	for _, action := range step.Actions {
		_, exists := signal.markets[action.Symbol]

		if !exists {
			return fmt.Errorf("signal: unknown symbol %q", action.Symbol)
		}

		switch action.Kind {
		case MoveMid:
			signal.shift(action.Symbol, action.Ticks, action.Ticks)
		case Trade:
			if action.Qty <= 0 || action.Side != "buy" && action.Side != "sell" {
				return fmt.Errorf("signal: valid trade side and quantity required")
			}

			if _, duplicate := trades[action.Symbol]; duplicate {
				return fmt.Errorf("signal: one trade per symbol and step supported")
			}

			trades[action.Symbol] = action
		case Add:
			if action.Price <= 0 || action.Qty <= 0 || !validSide(action.Side) {
				return fmt.Errorf("signal: valid add side, price, and quantity required")
			}

			signal.add(action.Symbol, action.Side, action.Price, action.Qty)
		case Cancel:
			if action.OrderID == "" || !signal.cancel(action.Symbol, action.OrderID) {
				return fmt.Errorf("signal: resting order %q not found", action.OrderID)
			}
		case Refill:
			if action.Qty <= 0 || !validSide(action.Side) {
				return fmt.Errorf("signal: valid refill side and quantity required")
			}

			signal.refill(action.Symbol, action.Side, action.Qty)
		case WidenSpread:
			if action.Ticks <= 0 {
				return fmt.Errorf("signal: positive spread widening required")
			}

			signal.shift(action.Symbol, -action.Ticks, action.Ticks)
		default:
			return fmt.Errorf("signal: unknown action kind %d", action.Kind)
		}
	}

	for symbol, trade := range trades {
		_, err := signal.execute(symbol, trade.Side, trade.Qty)

		if err != nil {
			return err
		}
	}

	samples := make([]Sample, len(signal.symbols))

	for index, symbol := range signal.symbols {
		_, traded := trades[symbol]
		samples[index] = signal.sample(symbol, traded)
	}

	signal.tape = [][]Sample{samples}
	return nil
}

/*
tradeSample derives the ready wire values from an explicit touch execution.
*/
func (signal *Signal) sample(
	symbol string,
	traded bool,
) Sample {
	market := signal.markets[symbol]
	vwap := market.notional / market.volume
	bids := append([]Order(nil), market.bids...)
	asks := append([]Order(nil), market.asks...)

	return Sample{
		Symbol:     symbol,
		Side:       market.lastSide,
		Price:      market.mid,
		TradePrice: market.last,
		Volume:     market.lastQty,
		TradeID:    market.tradeID,
		BookVolume: bids[0].Qty,
		At:         signal.at,
		Traded:     traded,
		Bids:       bids,
		Asks:       asks,
		Statistics: Statistics{
			Open:   market.open,
			High:   market.high,
			Low:    market.low,
			Volume: market.volume,
			VWAP:   vwap,
		},
	}
}

/*
seed creates the initial two-level resting book for one simulated symbol.
*/
func (signal *Signal) seed(symbol string) {
	market := signal.markets[symbol]
	quantity := idleVolume * 1000

	for level := int64(1); level <= 2; level++ {
		signal.add(symbol, "buy", market.mid-float64(level)*PriceIncrement, quantity)
		signal.add(symbol, "sell", market.mid+float64(level)*PriceIncrement, quantity)
	}
}

/*
add creates a uniquely identified resting order in the authoritative book.
*/
func (signal *Signal) add(symbol, side string, price, quantity float64) {
	signal.nextID++
	order := Order{
		ID:    fmt.Sprintf("SIM-%s-%s-%d", symbol, side, signal.nextID),
		Side:  side,
		Price: round(price),
		Qty:   round(quantity),
		At:    signal.at,
	}
	market := signal.markets[symbol]

	if side == "buy" {
		market.bids = append(market.bids, order)
		return
	}

	market.asks = append(market.asks, order)
}

/*
cancel removes one exact resting identity without disturbing unrelated orders.
*/
func (signal *Signal) cancel(symbol, orderID string) bool {
	market := signal.markets[symbol]

	for _, book := range []*[]Order{&market.bids, &market.asks} {
		for index, order := range *book {
			if order.ID != orderID {
				continue
			}

			*book = append((*book)[:index], (*book)[index+1:]...)
			return true
		}
	}

	return false
}

/*
refill adds quantity to the current touch while preserving its order identity.
*/
func (signal *Signal) refill(symbol, side string, quantity float64) {
	orders := signal.side(symbol, side)
	index := touchIndex(orders, side)
	orders[index].Qty = round(orders[index].Qty + quantity)
	orders[index].At = signal.at
}

/*
shift moves one or both sides in ticks and assigns new identities to the newly
resting prices, making the resulting L3 cancellation/addition causal.
*/
func (signal *Signal) shift(symbol string, bidTicks, askTicks int64) {
	market := signal.markets[symbol]
	bids := append([]Order(nil), market.bids...)
	asks := append([]Order(nil), market.asks...)
	market.bids = nil
	market.asks = nil

	for _, order := range bids {
		signal.add(symbol, "buy", order.Price+float64(bidTicks)*PriceIncrement, order.Qty)
	}

	for _, order := range asks {
		signal.add(symbol, "sell", order.Price+float64(askTicks)*PriceIncrement, order.Qty)
	}

	market.mid += float64(bidTicks+askTicks) * PriceIncrement / 2
}

/*
execute consumes the aggressed touch and records the resulting ticker session.
*/
func (signal *Signal) execute(symbol, side string, quantity float64) (float64, error) {
	bookSide := "sell"

	if side == "sell" {
		bookSide = "buy"
	}

	orders := signal.side(symbol, bookSide)
	index := touchIndex(orders, bookSide)

	if quantity > orders[index].Qty {
		return 0, fmt.Errorf("signal: %s trade quantity exceeds touch liquidity", symbol)
	}

	price := orders[index].Price
	orders[index].Qty = round(orders[index].Qty - quantity)
	orders[index].At = signal.at
	market := signal.markets[symbol]
	market.volume += quantity
	market.notional += price * quantity

	if market.open == 0 {
		market.open = price
		market.high = price
		market.low = price
	}

	market.high = max(market.high, price)
	market.low = min(market.low, price)
	market.lastSide = side
	market.last = price
	market.lastQty = quantity
	signal.nextTrade++
	market.tradeID = signal.nextTrade
	return price, nil
}

/*
side exposes the mutable authoritative side selected by an aggressor action.
*/
func (signal *Signal) side(symbol, side string) []Order {
	if side == "buy" {
		return signal.markets[symbol].bids
	}

	return signal.markets[symbol].asks
}

/*
touchIndex locates the economically best resting order on one side.
*/
func touchIndex(orders []Order, side string) int {
	best := 0

	for index := 1; index < len(orders); index++ {
		if side == "buy" && orders[index].Price > orders[best].Price ||
			side == "sell" && orders[index].Price < orders[best].Price {
			best = index
		}
	}

	return best
}

/*
validSide accepts only Kraken's buy and sell wire values.
*/
func validSide(side string) bool {
	return side == "buy" || side == "sell"
}

/*
round retains the eight decimal places represented by the fixture templates.
*/
func round(value float64) float64 {
	return math.Round(value*1e8) / 1e8
}
