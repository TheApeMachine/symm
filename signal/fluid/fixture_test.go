package fluid

import (
	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/theapemachine/symm/kraken"
)

type symbolBookFixture struct {
	symbol string
	seq    int
}

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
