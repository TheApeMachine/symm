package signal

import (
	"fmt"
	"math"
	"time"
)

/*
sample derives ready fixture values from the authoritative symbol state.
*/
func (market *symbolState) sample(
	symbol string,
	at time.Time,
	fills []Fill,
	bookChanged bool,
	touchChanged bool,
) Sample {
	vwap := market.notional / market.volume
	bids := append([]Order(nil), market.bids...)
	asks := append([]Order(nil), market.asks...)

	return Sample{
		Symbol:       symbol,
		Side:         market.lastSide,
		Price:        market.mid(),
		TradePrice:   market.last,
		Volume:       market.lastQty,
		TradeID:      market.tradeID,
		At:           at,
		Traded:       len(fills) > 0,
		Fills:        append([]Fill(nil), fills...),
		BookChanged:  bookChanged,
		TouchChanged: touchChanged,
		Bids:         bids,
		Asks:         asks,
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
func (market *symbolState) seed(
	symbol string,
	center float64,
	nextID *uint64,
	at time.Time,
) {
	for level := range bookLevels {
		ticks := int64(bestQuoteTicks + level)
		market.add(
			symbol, "buy", center-float64(ticks)*PriceIncrement,
			initialOrderQuantity, nextID, at,
		)
		market.add(
			symbol, "sell", center+float64(ticks)*PriceIncrement,
			initialOrderQuantity, nextID, at,
		)
	}
}

/*
add creates a uniquely identified resting order in the authoritative book.
*/
func (market *symbolState) add(
	symbol string,
	side string,
	price float64,
	quantity float64,
	nextID *uint64,
	at time.Time,
) {
	(*nextID)++
	order := Order{
		ID:       fmt.Sprintf("SIM-%s-%s-%d", symbol, side, *nextID),
		Side:     side,
		Price:    round(price),
		Qty:      round(quantity),
		Priority: *nextID,
		At:       at,
	}

	if side == "buy" {
		market.bids = append(market.bids, order)
		return
	}

	market.asks = append(market.asks, order)
}

/*
cancel removes one exact resting identity without disturbing unrelated orders.
*/
func (market *symbolState) cancel(orderID string) bool {
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
func (market *symbolState) refill(
	side string,
	quantity float64,
	at time.Time,
) error {
	orders := market.side(side)

	if len(orders) == 0 {
		return fmt.Errorf("signal: cannot refill empty %s side", side)
	}

	index := touchIndex(orders, side)
	orders[index].Qty = round(orders[index].Qty + quantity)
	orders[index].At = at
	return nil
}

/*
shift moves one or both sides in ticks and assigns new identities to the newly
resting prices, making the resulting L3 cancellation/addition causal.
*/
func (market *symbolState) shift(
	symbol string,
	nextID *uint64,
	at time.Time,
	bidTicks int64,
	askTicks int64,
) {
	if bidTicks == 0 && askTicks == 0 {
		return
	}

	bids := append([]Order(nil), market.bids...)
	asks := append([]Order(nil), market.asks...)
	market.bids = nil
	market.asks = nil

	for _, order := range bids {
		market.add(
			symbol, "buy", order.Price+float64(bidTicks)*PriceIncrement,
			order.Qty, nextID, at,
		)
	}

	for _, order := range asks {
		market.add(
			symbol, "sell", order.Price+float64(askTicks)*PriceIncrement,
			order.Qty, nextID, at,
		)
	}

}

/*
execute consumes the aggressed touch and records the resulting ticker session.
*/
func (market *symbolState) execute(
	side string,
	quantity float64,
	at time.Time,
	nextTrade *uint64,
) ([]Fill, error) {
	bookSide := "sell"

	if side == "sell" {
		bookSide = "buy"
	}

	orders := market.side(bookSide)

	if len(orders) == 0 {
		return nil, fmt.Errorf("signal: no %s liquidity", bookSide)
	}

	available := 0.0

	for _, order := range orders {
		available += order.Qty
	}

	if quantity >= available {
		return nil, fmt.Errorf("signal: trade must leave resting %s liquidity", bookSide)
	}

	fills := []Fill{}
	remaining := quantity

	for remaining > 0 {
		orders = market.side(bookSide)
		index := touchIndex(orders, bookSide)
		filled := min(remaining, orders[index].Qty)
		price := orders[index].Price
		orders[index].Qty = round(orders[index].Qty - filled)
		orders[index].At = at

		if orders[index].Qty == 0 {
			orders = append(orders[:index], orders[index+1:]...)
			market.replace(bookSide, orders)
		}

		market.volume += filled
		market.notional += price * filled
		market.record(side, price, filled)
		(*nextTrade)++
		market.tradeID = *nextTrade
		fills = append(fills, Fill{
			Side: side, Price: price, Qty: filled, TradeID: *nextTrade, At: at,
		})
		remaining = round(remaining - filled)
	}

	return fills, nil
}

/*
record updates the trade-derived ticker session for one actual fill.
*/
func (market *symbolState) record(side string, price float64, quantity float64) {

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
}

/*
mid derives the scenario center from the live authoritative touch.
*/
func (market *symbolState) mid() float64 {
	quote, exists := market.quote()

	if !exists {
		return 0
	}

	return (quote.Bid + quote.Ask) / 2
}

/*
validate rejects empty, crossed, off-tick, or non-positive draft books before
they can become fixture state.
*/
func (market *symbolState) validate() error {
	quote, exists := market.quote()

	if !exists {
		return fmt.Errorf("both resting sides required")
	}

	if quote.Bid >= quote.Ask {
		return fmt.Errorf("best bid must remain below best ask")
	}

	for _, orders := range [][]Order{market.bids, market.asks} {
		for _, order := range orders {
			if order.Qty <= 0 || !onTick(order.Price) {
				return fmt.Errorf("positive on-tick orders required")
			}
		}
	}

	return nil
}

/*
side exposes the mutable authoritative side selected by an aggressor action.
*/
func (market *symbolState) side(side string) []Order {
	if side == "buy" {
		return market.bids
	}

	return market.asks
}

/*
quote returns the executable touch when both authoritative sides are present.
*/
func (market *symbolState) quote() (Quote, bool) {
	if len(market.bids) == 0 || len(market.asks) == 0 {
		return Quote{}, false
	}

	return Quote{
		Bid:    market.bids[touchIndex(market.bids, "buy")].Price,
		BidQty: touchQuantity(market.bids, "buy"),
		Ask:    market.asks[touchIndex(market.asks, "sell")].Price,
		AskQty: touchQuantity(market.asks, "sell"),
	}, true
}

/*
replace commits a changed side after complete-touch removal.
*/
func (market *symbolState) replace(side string, orders []Order) {
	if side == "buy" {
		market.bids = orders
		return
	}

	market.asks = orders
}

/*
touchQuantity aggregates all FIFO orders resting at the best price.
*/
func touchQuantity(orders []Order, side string) float64 {
	price := orders[touchIndex(orders, side)].Price
	quantity := 0.0

	for _, order := range orders {
		if order.Price == price {
			quantity += order.Qty
		}
	}

	return quantity
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
onTick verifies that an explicit action price belongs to the simulated
instrument's configured price grid.
*/
func onTick(price float64) bool {
	ticks := price / PriceIncrement
	return math.Abs(ticks-math.Round(ticks)) < 1e-8
}

/*
round retains the eight decimal places represented by the fixture templates.
*/
func round(value float64) float64 {
	return math.Round(value*1e8) / 1e8
}
