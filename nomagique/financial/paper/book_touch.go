package paper

import (
	"github.com/krakenfx/api-go/v2/pkg/book"
	"github.com/krakenfx/api-go/v2/pkg/decimal"
)

/* marketBook owns the reconciled book and executions in its current L3 bracket. */
type marketBook struct {
	book    *book.Book
	matched [2]*decimal.Decimal
}

/* marketBook retains executions against the reconciled market. */
func (server *BookServer) marketBook(symbol string) *marketBook {
	market := server.markets[symbol]

	if market == nil {
		market = &marketBook{}
		server.markets[symbol] = market
	}
	return market
}

/* match accumulates exact executions at the currently observed side's touch. */
func (market *marketBook) match(side int, quantity *decimal.Decimal) {
	if market.matched[side] == nil {
		market.matched[side] = quantity.Copy()
		return
	}
	previous := market.matched[side]
	market.matched[side] = previous.SetScale(max(previous.GetScale(), quantity.GetScale())).Add(quantity)
}

/* touch copies only the two prices and quantities that the next L3 update replaces. */
func (market *marketBook) touch() [2][2]*decimal.Decimal {
	prior := [2][2]*decimal.Decimal{}
	for side, level := range []*book.Level{market.book.BestBid(), market.book.BestAsk()} {
		if level != nil {
			prior[side] = [2]*decimal.Decimal{level.Price.Copy(), level.Quantity.Copy()}
		}
	}
	return prior
}

/*
touch exposes prior touch, attributable executions and observed price geometry.
Slots 17/18 are prior quantity, 19/20 matched quantity capped by that quantity,
21/22 current quantity at the prior price, 23/24 retreat indicators, 25/26
non-retreat disposition indicators and 27/28 prior prices. Capping executions
is the physical attribution rule in LEGACY's order-flow toxicity specification:
executions beyond Q0 cannot be attributed to that previously displayed quantity.
*/
func (server *BookServer) touch(market *marketBook, prior [2][2]*decimal.Decimal) {
	for side, current := range []*book.Level{market.book.BestBid(), market.book.BestAsk()} {
		price, quantity := prior[side][0], prior[side][1]

		if current == nil || price == nil || quantity.Sign() <= 0 {
			continue
		}
		server.values[17+side], server.present[17+side] = quantity.Float64(), true
		server.values[27+side], server.present[27+side] = price.Float64(), true
		matched := market.matched[side]

		if matched != nil {
			attributed := matched

			if matched.Cmp(quantity) > 0 {
				attributed = quantity
			}
			server.values[19+side] = attributed.Float64()
		}
		server.present[19+side] = true
		direction := current.Price.Cmp(price)
		retreat := (side == 0 && direction < 0) || (side == 1 && direction > 0)
		server.present[23+side] = true

		if retreat {
			server.values[23+side] = 1
			server.present[21+side], server.present[25+side] = true, true
			continue
		}
		levels := market.book.Bids.Levels

		if side == 1 {
			levels = market.book.Asks.Levels
		}
		previousLevel := levels[price.String()]

		if previousLevel == nil {
			// An improved touch can push the old price outside subscribed depth.
			// Its disposition is then unknown, not a withdrawal of all its liquidity.
			continue
		}
		server.values[21+side], server.present[21+side] = previousLevel.Quantity.Float64(), true
		server.values[25+side], server.present[25+side] = 1, true
	}
}
