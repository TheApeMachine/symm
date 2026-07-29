package pumpdump_test

import (
	"testing"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/signal/pumpdump"
)

/*
TestCalculate proves pumpdump treats late ticker deliveries as non-observations
instead of failing the volume clock or poisoning peers in the same batch.
*/
func TestCalculate(t *testing.T) {
	Convey("Given a symbol with book state and executed volume", t, func() {
		signal := pumpdump.NewSignal(t.Context(), make(chan []byte, 8), 128)
		at := time.Date(2026, 7, 25, 0, 0, 0, 0, time.UTC)
		increment := decimal.NewFromFloat64(0.01)

		signal.Calculate(nil, nil, []kraken.BookData{
			{
				Symbol:         "SIM1/USD",
				Type:           "snapshot",
				PriceIncrement: increment,
				Bids: []kraken.BookLevel{
					{Price: *decimal.NewFromFloat64(99.99), Qty: 10},
				},
				Asks: []kraken.BookLevel{
					{Price: *decimal.NewFromFloat64(100.01), Qty: 10},
				},
				Timestamp: at,
			},
			{
				Symbol:         "SIM2/USD",
				Type:           "snapshot",
				PriceIncrement: increment,
				Bids: []kraken.BookLevel{
					{Price: *decimal.NewFromFloat64(49.99), Qty: 10},
				},
				Asks: []kraken.BookLevel{
					{Price: *decimal.NewFromFloat64(50.01), Qty: 10},
				},
				Timestamp: at,
			},
		})

		signal.Calculate(nil, []kraken.TradeData{
			{
				Symbol:    "SIM1/USD",
				Side:      "buy",
				Price:     *decimal.NewFromFloat64(100.01),
				Qty:       25,
				Timestamp: at,
			},
			{
				Symbol:    "SIM2/USD",
				Side:      "buy",
				Price:     *decimal.NewFromFloat64(50.01),
				Qty:       25,
				Timestamp: at,
			},
		}, nil)
		first := signal.Calculate([]kraken.TickerData{
			{
				Symbol:    "SIM1/USD",
				Last:      decimal.NewFromFloat64(100),
				Timestamp: at.Add(2 * time.Second),
			},
			{
				Symbol:    "SIM2/USD",
				Last:      decimal.NewFromFloat64(50),
				Timestamp: at.Add(2 * time.Second),
			},
		}, nil, nil)
		So(first, ShouldNotBeEmpty)

		Convey("A late ticker for one symbol should not fail the batch", func() {
			rows := signal.Calculate([]kraken.TickerData{
				{
					Symbol:    "SIM1/USD",
					Last:      decimal.NewFromFloat64(100),
					Timestamp: at.Add(time.Second),
				},
				{
					Symbol:    "SIM2/USD",
					Last:      decimal.NewFromFloat64(50),
					Timestamp: at.Add(3 * time.Second),
				},
			}, nil, nil)

			symbols := map[string]bool{}

			for _, measurement := range rows {
				symbols[measurement.Symbol] = true
			}

			So(symbols["SIM1/USD"], ShouldBeFalse)
			So(symbols["SIM2/USD"], ShouldBeTrue)
		})

		Convey("A later causal ticker should still advance the symbol", func() {
			rows := signal.Calculate([]kraken.TickerData{
				{
					Symbol:    "SIM1/USD",
					Last:      decimal.NewFromFloat64(100),
					Timestamp: at.Add(4 * time.Second),
				},
			}, nil, nil)
			So(rows, ShouldNotBeEmpty)
			So(rows[0].Symbol, ShouldEqual, "SIM1/USD")
		})
	})
}
