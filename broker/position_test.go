package broker_test

import (
	"context"
	"testing"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/krakenfx/api-go/v2/pkg/spot"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/kraken"
)

/*
The band walk closes a virtual lot when the mark has moved two percent from
the entry price. The fee legs are tier rates, and a round trip costs more than
the band, so the walk cannot be measured on the fee-net return: a lot that did
not move stays open.
*/
func TestShouldExitWalksTwoPercentFromTheEntryPrice(t *testing.T) {
	t.Parallel()

	price := broker.NewPrice(context.Background(), nil, nil, nil, spot.NewNormalizer())

	symbol := "AAVE/USD"
	tier := decimal.NewFromFloat64(1.587)
	price.SetFee(symbol, kraken.TradeVolumeFee{Fee: tier, Minfee: tier, Maxfee: tier})

	position := broker.NewPosition(symbol, "R01/R02", true, price)
	position.EntryPrice = decimal.NewFromFloat64(0.5)

	// The mark that follows the entry.
	position.Update(decimal.NewFromFloat64(0.5))

	if position.ShouldExit() {
		t.Fatal("a lot that did not move closed on the band walk")
	}

	position.Update(decimal.NewFromFloat64(0.505))

	if position.ShouldExit() {
		t.Fatal("a one percent move closed the band walk")
	}

	position.Update(decimal.NewFromFloat64(0.5125))

	if !position.ShouldExit() {
		t.Fatal("a two and a half percent rally did not close the band walk")
	}

	position.Update(decimal.NewFromFloat64(0.4875))

	if !position.ShouldExit() {
		t.Fatal("a two and a half percent drop did not close the band walk")
	}
}

/*
A paper lot leaves the desk on the action tree, not on the band walk.
*/
func TestShouldExitIgnoresPaperPositions(t *testing.T) {
	t.Parallel()

	price := broker.NewPrice(context.Background(), nil, nil, nil, spot.NewNormalizer())

	position := broker.NewPosition("AAVE/USD", "R01/R02", false, price)
	position.EntryPrice = decimal.NewFromFloat64(0.5)
	position.Update(decimal.NewFromFloat64(0.4))

	if position.ShouldExit() {
		t.Fatal("a paper position closed on the virtual band walk")
	}
}
