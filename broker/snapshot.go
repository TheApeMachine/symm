package broker

import (
	"fmt"
	"slices"

	"github.com/bytedance/sonic"
	spotbook "github.com/krakenfx/api-go/v2/pkg/book"
	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/data"
	"github.com/theapemachine/symm/kraken"
)

/*
Level is one price level of a stamped book side.
*/
type Level struct {
	Price    *decimal.Decimal `json:"price"`
	Quantity *decimal.Decimal `json:"quantity"`
}

/*
Snapshot is a symbol's resident book as ingress stamped it onto an event: every
level of both sides inside the book's depth window, best first, and the window
size itself. It is taken under the symbol's read lock at the event's sequence
number, so it is exactly the book a SeqIdx-ordered replay of the stored level3
tape rebuilds at that event. A snapshot always holds an executable touch: an
unseeded, diverged, missing, one-sided, or crossed book is never stamped.

Every consumer of book state reads the snapshot its event carries, never the
live book, so a consumer that runs later than ingress still sees the book the
event happened against.
*/
type Snapshot struct {
	Depth int     `json:"depth"`
	Bids  []Level `json:"bids"`
	Asks  []Level `json:"asks"`
}

/*
Touch returns the snapshot's best bid and best ask.
*/
func (snapshot *Snapshot) Touch() (bid Level, ask Level) {
	return snapshot.Bids[0], snapshot.Asks[0]
}

/*
Full reports whether a side fills the depth window, so prices beyond its worst
level may rest outside the window rather than be absent from the book.
*/
func (snapshot *Snapshot) Full(levels []Level) bool {
	return len(levels) == snapshot.Depth
}

/*
Equal reports whether two snapshots hold the same window and the same levels to
the venue's exact digits.
*/
func (snapshot *Snapshot) Equal(other *Snapshot) bool {
	if snapshot == nil || other == nil {
		return snapshot == other
	}

	if snapshot.Depth != other.Depth {
		return false
	}

	return sameLevels(snapshot.Bids, other.Bids) && sameLevels(snapshot.Asks, other.Asks)
}

func sameLevels(left, right []Level) bool {
	if len(left) != len(right) {
		return false
	}

	for index := range left {
		if left[index].Price.Cmp(right[index].Price) != 0 {
			return false
		}

		if left[index].Quantity.Cmp(right[index].Quantity) != 0 {
			return false
		}
	}

	return true
}

/*
Symbols returns the distinct symbols of trades, in frame order.
*/
func Symbols(trades []kraken.TradeData) []string {
	symbols := make([]string, 0, len(trades))

	for _, trade := range trades {
		if trade.Symbol != "" && !slices.Contains(symbols, trade.Symbol) {
			symbols = append(symbols, trade.Symbol)
		}
	}

	return symbols
}

/*
SymbolLayer returns the layer indexed exactly "<kind>/<symbol>" (book, price,
instrument), so BTC/USD never matches BTC/USDT.
*/
func SymbolLayer(value *data.Value, kind, symbol string) []byte {
	key := kind + "/" + symbol

	for index, layer := range data.Indexed[[]byte](value, key) {
		if index == key {
			return layer
		}
	}

	return nil
}

/*
ReadSnapshot decodes the book of symbol stamped onto value (the spotbook.Book
that Book.JSON marshals). A tick that carries no book for symbol is (nil, nil).
A book layer that does not decode, or is one-sided, is an error.

Levels come from each side's Levels map, sorted best first: the SDK's
High/Low/Lower/Higher links are not serialized, so walking them from a decoded
book yields only the touch.
*/
func ReadSnapshot(value *data.Value, symbol string) (*Snapshot, error) {
	layer := SymbolLayer(value, "book", symbol)

	if layer == nil {
		return nil, nil
	}

	var book spotbook.Book

	if err := sonic.Unmarshal(layer, &book); err != nil {
		return nil, errnie.Err(
			errnie.UnprocessableContent,
			"broker: book/"+symbol+" layer does not decode",
			err,
		)
	}

	bids, err := sideLevels(symbol, book.Bids, -1)

	if err != nil {
		return nil, err
	}

	asks, err := sideLevels(symbol, book.Asks, 1)

	if err != nil {
		return nil, err
	}

	if len(bids) == 0 || len(asks) == 0 {
		return nil, errnie.Err(
			errnie.Validation,
			fmt.Sprintf("broker: book/%s is one-sided (%d bids, %d asks)", symbol, len(bids), len(asks)),
			nil,
		)
	}

	return &Snapshot{
		Depth: book.MaxDepth,
		Bids:  bids,
		Asks:  asks,
	}, nil
}

/*
sideLevels returns a decoded side's levels ordered by price: order -1 is
descending (bids, best first), 1 is ascending (asks, best first).
*/
func sideLevels(symbol string, side *spotbook.Side, order int) ([]Level, error) {
	if side == nil {
		return nil, nil
	}

	levels := make([]Level, 0, len(side.Levels))

	for key, level := range side.Levels {
		if level == nil || level.Price == nil || level.Quantity == nil {
			return nil, errnie.Err(
				errnie.Validation,
				fmt.Sprintf("broker: book/%s level %s has no price or quantity", symbol, key),
				nil,
			)
		}

		levels = append(levels, Level{Price: level.Price, Quantity: level.Quantity})
	}

	slices.SortFunc(levels, func(left, right Level) int {
		return order * left.Price.Cmp(right.Price)
	})

	return levels, nil
}
