package manifold

import (
	"context"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/spf13/viper"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/logic"
	"github.com/theapemachine/symm/market"
)

func setManifoldTestViper() {
	viper.Set("signals.manifold.measurements_capacity", 16)
	viper.Set("signals.manifold.tick_size", 0.01)
	viper.Set("signals.manifold.grid_half_width", 8)
	viper.Set("signals.manifold.grid_x", 16)
	viper.Set("signals.manifold.grid_y", 1)
	viper.Set("signals.manifold.grid_z", 8)
	viper.Set("signals.manifold.max_modes", 8)
	viper.Set("signals.manifold.integration_interval", "100ms")
	viper.Set("market.book_depth_levels", 4)
}

func TestSignalObserveBooks(t *testing.T) {
	Convey("Given a typed manifold signal", t, func() {
		setManifoldTestViper()
		signal := NewSignal(context.Background())
		defer signal.Close()

		signal.field.RegisterSymbols([]string{"XBT/USD"})
		eventAt := time.Date(2026, 7, 5, 12, 0, 0, 0, time.UTC)

		Convey("When book rows are observed", func() {
			err := signal.observeBooks(kraken.BookDataSlice{{
				Symbol:    "XBT/USD",
				Timestamp: eventAt,
				Bids:      []kraken.BookLevel{{Price: 49990, Qty: 1}},
				Asks:      []kraken.BookLevel{{Price: 50010, Qty: 1}},
			}})
			state := signal.field.universe.loadSymbol("XBT/USD")

			Convey("It should feed the field without tree artifacts", func() {
				So(err, ShouldBeNil)
				So(state, ShouldNotBeNil)
				So(state.bookReady, ShouldBeTrue)
				So(state.midPrice, ShouldBeGreaterThan, 0)
			})
		})
	})
}

func TestSignalObserveTickers(t *testing.T) {
	Convey("Given a typed manifold signal", t, func() {
		setManifoldTestViper()
		signal := NewSignal(context.Background())
		defer signal.Close()

		signal.field.RegisterSymbols([]string{"BTC/USD"})
		eventAt := time.Date(2026, 7, 5, 12, 0, 0, 0, time.UTC)

		Convey("When ticker rows are observed", func() {
			err := signal.observeTickers(kraken.TickerDataSlice{{
				Symbol:    "BTC/USD",
				Last:      50000,
				Bid:       49990,
				Ask:       50010,
				Timestamp: eventAt,
			}})
			state := signal.field.universe.loadSymbol("BTC/USD")

			Convey("It should feed the field without tree artifacts", func() {
				So(err, ShouldBeNil)
				So(state, ShouldNotBeNil)
				So(state.lastPrice, ShouldEqual, 50000)
			})
		})
	})
}

func TestSignalMeasure(t *testing.T) {
	Convey("Given a manifold signal with typed market state", t, func() {
		setManifoldTestViper()
		signal := NewSignal(context.Background())
		defer signal.Close()

		eventAt := time.Date(2026, 7, 5, 12, 0, 0, 0, time.UTC)
		signal.field.RegisterSymbols([]string{"BTC/USD"})
		So(signal.observeBooks(bookRows("BTC/USD", eventAt)), ShouldBeNil)
		So(signal.observeTrades(tradeRows("BTC/USD", eventAt)), ShouldBeNil)

		Convey("When ticker input is measured", func() {
			measurements, err := signal.Measure(market.Input{
				Role:   "ticker",
				At:     eventAt,
				Ticker: tickerRows("BTC/USD", eventAt),
			}, nil)

			Convey("It should emit typed manifold measurements", func() {
				So(err, ShouldBeNil)
				So(measurements, ShouldNotBeEmpty)
				So(measurements[0].Source, ShouldEqual, logic.SourceManifold)
				So(measurements[0].Symbol, ShouldEqual, "BTC/USD")
				So(measurements[0].Confidence, ShouldBeGreaterThan, 0)
				So(measurements[0].HasDistribution(), ShouldBeTrue)
			})
		})
	})
}

func BenchmarkSignalMeasure(b *testing.B) {
	setManifoldTestViper()
	eventAt := time.Date(2026, 7, 5, 12, 0, 0, 0, time.UTC)

	b.ReportAllocs()

	for b.Loop() {
		signal := NewSignal(context.Background())
		signal.field.RegisterSymbols([]string{"BTC/USD"})

		if err := signal.observeBooks(bookRows("BTC/USD", eventAt)); err != nil {
			b.Fatal(err)
		}

		if err := signal.observeTrades(tradeRows("BTC/USD", eventAt)); err != nil {
			b.Fatal(err)
		}

		measurements, err := signal.Measure(market.Input{
			Role:   "ticker",
			At:     eventAt,
			Ticker: tickerRows("BTC/USD", eventAt),
		}, nil)

		if err != nil {
			b.Fatal(err)
		}

		if len(measurements) == 0 {
			b.Fatal("Measure returned no measurements")
		}

		_ = signal.Close()
	}
}

func bookRows(symbol string, eventAt time.Time) kraken.BookDataSlice {
	return kraken.BookDataSlice{{
		Symbol:    symbol,
		Timestamp: eventAt,
		Bids: []kraken.BookLevel{
			{Price: 49990, Qty: 1.2},
			{Price: 49980, Qty: 1.0},
		},
		Asks: []kraken.BookLevel{
			{Price: 50010, Qty: 0.8},
			{Price: 50020, Qty: 1.1},
		},
	}}
}

func tradeRows(symbol string, eventAt time.Time) kraken.TradeDataSlice {
	return kraken.TradeDataSlice{{
		Symbol:    symbol,
		Side:      "buy",
		Price:     50000,
		Qty:       0.4,
		Timestamp: eventAt,
	}}
}

func tickerRows(symbol string, eventAt time.Time) kraken.TickerDataSlice {
	return kraken.TickerDataSlice{{
		Symbol:    symbol,
		Last:      50000,
		Bid:       49990,
		Ask:       50010,
		Volume:    100,
		Timestamp: eventAt,
	}}
}
