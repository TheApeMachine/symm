package kraken

import (
	"github.com/krakenfx/api-go/v2/pkg/decimal"
)

/*
Side identifies which half of an order book a level belongs to.
*/
type Side string

const (
	SideBid Side = "bid"
	SideAsk Side = "ask"
)

/*
BookOrder is one resident L3 order used by bounded per-position execution
state. It retains Kraken's exact fixed-point LimitPrice and OrderQty so
executable-liquidation arithmetic never leaves Kraken's decimal space. OrderID
is the exchange order identity that survives add/modify/delete across frames.

This is identity state for one tracked execution surface, not a general-purpose
exchange book: there is no resident book per market, no registry, and no
market-wide owner.
*/
type BookOrder struct {
	OrderID    string
	LimitPrice *decimal.Decimal
	OrderQty   *decimal.Decimal
}
