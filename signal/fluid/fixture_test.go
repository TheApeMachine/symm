package fluid

import (
	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/theapemachine/symm/kraken"
)

/*
symbolBookFixture provides reusable book state for fluid tests so multi-step
grid behavior is exercised consistently.
*/
type symbolBookFixture struct {
	symbol string
	seq    int
}

/*
snapshot builds a deterministic book snapshot for fluid tests so grid
transitions use realistic bid and ask levels.
*/
func (fixture *symbolBookFixture) snapshot(
	bidPrice, bidQty, askPrice, askQty float64,
) kraken.BookData {
	fixture.seq++

	feedType := "update"

	if fixture.seq == 1 {
		feedType = "snapshot"
	}

	return kraken.BookData{
		Symbol: fixture.symbol,
		Type:   feedType,
		Bids: []kraken.BookLevel{
			{Price: *decimal.NewFromFloat64(bidPrice), Qty: bidQty},
			{Price: *decimal.NewFromFloat64(bidPrice - 0.01), Qty: bidQty},
		},
		Asks: []kraken.BookLevel{
			{Price: *decimal.NewFromFloat64(askPrice), Qty: askQty},
			{Price: *decimal.NewFromFloat64(askPrice + 0.01), Qty: askQty},
		},
	}
}
