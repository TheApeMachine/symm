package fluid

import "github.com/theapemachine/symm/kraken"

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
			{Price: bidPrice, Qty: bidQty},
			{Price: bidPrice - 0.01, Qty: bidQty},
		},
		Asks: []kraken.BookLevel{
			{Price: askPrice, Qty: askQty},
			{Price: askPrice + 0.01, Qty: askQty},
		},
	}
}
