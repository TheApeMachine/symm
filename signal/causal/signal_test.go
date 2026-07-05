package causal

import (
	"context"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/types"
)

var _ types.Signal[kraken.TickerData] = (*Signal[kraken.TickerData])(nil)
var _ types.Signal[kraken.BookData] = (*Signal[kraken.BookData])(nil)
var _ types.Signal[kraken.TradeData] = (*Signal[kraken.TradeData])(nil)

func TestSignalIngestRoles(t *testing.T) {
	Convey("Given a causal signal", t, func() {
		signal := NewSignal[kraken.TickerData](context.Background())
		defer signal.Close()

		Convey("It declares the Kraken ingest roles", func() {
			So(signal.IngestRoles(), ShouldResemble, []string{"ticker", "book", "trade"})
		})
	})
}

func TestSignalCategories(t *testing.T) {
	Convey("Given a causal signal", t, func() {
		signal := NewSignal[kraken.TickerData](context.Background())
		defer signal.Close()

		Convey("It returns the causal category types", func() {
			So(signal.Categories(), ShouldResemble, []types.CategoryType{
				types.EndogenousAlpha,
				types.SystemicBeta,
				types.LiquidityShock,
				types.CausalNoise,
			})
		})
	})
}

func TestSignalMeasure(t *testing.T) {
	Convey("Given typed Kraken rows", t, func() {
		tickerSignal := NewSignal[kraken.TickerData](context.Background())
		bookSignal := NewSignal[kraken.BookData](context.Background())
		tradeSignal := NewSignal[kraken.TradeData](context.Background())
		defer tickerSignal.Close()
		defer bookSignal.Close()
		defer tradeSignal.Close()

		base := time.Date(2026, 5, 30, 12, 0, 0, 0, time.UTC)

		tickerMeasurements, tickerErr := tickerSignal.Measure(kraken.TickerData{
			Symbol:    "BTC/USD",
			Bid:       99,
			Ask:       101,
			Timestamp: base,
		}, nil)

		So(tickerErr, ShouldBeNil)
		So(tickerMeasurements, ShouldBeNil)

		for index := range 8 {
			price := 100 + float64(index)
			_, tickerErr = tickerSignal.Measure(kraken.TickerData{
				Symbol:    "BTC/USD",
				Last:      price,
				Bid:       price - 0.01,
				Ask:       price + 0.01,
				BidQty:    10,
				AskQty:    10,
				ChangePct: 0.1,
				Timestamp: base.Add(time.Duration(index) * time.Second),
			}, nil)
			So(tickerErr, ShouldBeNil)

			_, bookErr := bookSignal.Measure(kraken.BookData{
				Symbol:    "BTC/USD",
				Type:      "update",
				Timestamp: base.Add(time.Duration(index) * time.Second),
				Bids: []kraken.BookLevel{{
					Price: price - 0.01,
					Qty:   10,
				}},
				Asks: []kraken.BookLevel{{
					Price: price + 0.01,
					Qty:   10,
				}},
			}, nil)
			So(bookErr, ShouldBeNil)

			_, tradeErr := tradeSignal.Measure(kraken.TradeData{
				Symbol:    "BTC/USD",
				Side:      "buy",
				Price:     price,
				Qty:       1 + float64(index),
				Timestamp: base.Add(time.Duration(index) * time.Second),
			}, nil)
			So(tradeErr, ShouldBeNil)
		}
	})
}

func BenchmarkSignalMeasure(b *testing.B) {
	signal := NewSignal[kraken.TradeData](context.Background())
	defer signal.Close()

	base := time.Date(2026, 5, 30, 12, 0, 0, 0, time.UTC)

	b.ReportAllocs()

	for b.Loop() {
		for index := range 8 {
			_, _ = signal.Measure(kraken.TradeData{
				Symbol:    "BTC/USD",
				Side:      "buy",
				Price:     100 + float64(index),
				Qty:       1 + float64(index),
				Timestamp: base.Add(time.Duration(index) * time.Second),
			}, nil)
		}
	}
}
