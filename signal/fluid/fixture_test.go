package fluid

type symbolBookFixture struct {
	symbol string
	seq    int
}

func (fixture *symbolBookFixture) snapshot(
	bidPrice, bidQty, askPrice, askQty float64,
) BookUpdate {
	fixture.seq++

	feedType := "update"

	if fixture.seq == 1 {
		feedType = "snapshot"
	}

	return BookUpdate{
		Symbol: fixture.symbol,
		Type:   feedType,
		Bids: []BookLevel{
			{Price: bidPrice, Qty: bidQty},
			{Price: bidPrice - 0.01, Qty: bidQty},
		},
		Asks: []BookLevel{
			{Price: askPrice, Qty: askQty},
			{Price: askPrice + 0.01, Qty: askQty},
		},
	}
}
