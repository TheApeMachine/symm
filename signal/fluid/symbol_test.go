package fluid

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/spf13/viper"
	"github.com/theapemachine/symm/kraken/market"
	"github.com/theapemachine/symm/market/perspectives"
	"github.com/theapemachine/symm/numeric/adaptive"
)

func fluidTestCategories() map[string]perspectives.CategoryType {
	return map[string]perspectives.CategoryType{
		"laminar":   perspectives.CategoryLaminar,
		"inertial":  perspectives.CategoryInertial,
		"viscous":   perspectives.CategoryViscous,
		"turbulent": perspectives.CategoryTurbulent,
	}
}

func fluidTestClassifier() *adaptive.Classifier {
	return adaptive.NewClassifier(
		fluidDefaultBandEdges,
		[]float64{0, 1, 2, 3},
		[]string{"laminar", "inertial", "viscous", "turbulent"},
	)
}

type symbolBookFixture struct {
	symbol string
}

func (fixture symbolBookFixture) snapshot(
	bidPrice, bidQty, askPrice, askQty float64,
) market.Book {
	bids := []market.BookLevel{{Price: bidPrice, Qty: bidQty}}
	asks := []market.BookLevel{{Price: askPrice, Qty: askQty}}

	update := market.Book{
		Symbol: fixture.symbol,
		Bids:   bids,
		Asks:   asks,
	}
	update.Checksum = update.ComputedChecksum()
	update.SetEnvelopeType(market.BookSnapshot)

	return update
}

func TestFluidSymbolRejectsDeltaBeforeSnapshot(t *testing.T) {
	Convey("Given a fluid symbol fed a delta before any snapshot", t, func() {
		symbol := "ETH/EUR"
		viper.Set("market.book_depth_levels", 10)
		state, err := NewFluidSymbol(symbol, fluidTestClassifier())
		So(err, ShouldBeNil)
		fixture := symbolBookFixture{symbol: symbol}

		delta := fixture.snapshot(99, 10, 101, 6)
		delta.SetEnvelopeType("update")
		state.FeedBook(delta)

		Convey("It should not treat the book as ready", func() {
			So(state.HasBook(), ShouldBeFalse)
		})

		Convey("It should report Measure as not ready", func() {
			measurement, _, err := state.Measure(fluidTestCategories())

			So(err, ShouldBeNil)
			So(measurement.Source, ShouldEqual, perspectives.SourceNone)
		})
	})
}

func TestFluidSymbolMeasureSkipsDivergedBook(t *testing.T) {
	Convey("Given a fluid symbol with a verified book", t, func() {
		symbol := "ETH/EUR"
		viper.Set("market.book_depth_levels", 10)
		state, err := NewFluidSymbol(symbol, fluidTestClassifier())
		So(err, ShouldBeNil)
		fixture := symbolBookFixture{symbol: symbol}

		state.FeedTicker(market.TickerUpdate{
			Symbol: symbol, Last: 100, Bid: 99, Ask: 101, Volume: 1000,
		})
		state.FeedBook(fixture.snapshot(99, 10, 101, 6))

		measurement, _, err := state.Measure(fluidTestCategories())

		Convey("It should publish a field reading", func() {
			So(err, ShouldBeNil)
			So(measurement.Source, ShouldNotBeEmpty)
		})

		Convey("When the maintained book diverges from the exchange checksum", func() {
			badDelta := fixture.snapshot(98, 10, 101, 6)
			badDelta.SetEnvelopeType("update")
			badDelta.Checksum = 1
			state.FeedBook(badDelta)

			measurement, _, err := state.Measure(fluidTestCategories())

			Convey("It should suppress field emission", func() {
				So(err, ShouldBeNil)
				So(measurement.Source, ShouldEqual, perspectives.SourceNone)
			})

			Convey("It should suppress dashboard rows", func() {
				So(state.Row(), ShouldBeNil)
			})
		})
	})
}

func TestFluidSymbolMeasureLaminarField(t *testing.T) {
	Convey("Given a balanced book with no Reynolds activity", t, func() {
		symbol := "BTC/EUR"
		viper.Set("market.book_depth_levels", 10)
		state, err := NewFluidSymbol(symbol, fluidTestClassifier())
		So(err, ShouldBeNil)
		fixture := symbolBookFixture{symbol: symbol}

		state.FeedTicker(market.TickerUpdate{
			Symbol: symbol, Last: 100, Bid: 100, Ask: 100, Volume: 1000,
		})
		state.FeedBook(fixture.snapshot(100, 5, 100, 5))

		measurement, _, err := state.Measure(fluidTestCategories())

		Convey("It should still publish a laminar reading", func() {
			So(err, ShouldBeNil)
			So(measurement.Source, ShouldEqual, perspectives.SourceFluid)
			So(measurement.Category, ShouldEqual, perspectives.CategoryLaminar)
		})
	})
}

func BenchmarkFluidSymbolMeasure(b *testing.B) {
	symbol := "ETH/EUR"
	viper.Set("market.book_depth_levels", 10)
	state, err := NewFluidSymbol(symbol, fluidTestClassifier())

	if err != nil {
		b.Fatal(err)
	}
	fixture := symbolBookFixture{symbol: symbol}

	state.FeedTicker(market.TickerUpdate{
		Symbol: symbol, Last: 100, Bid: 99, Ask: 101, Volume: 1000,
	})
	state.FeedBook(fixture.snapshot(99, 10, 101, 6))

	b.ReportAllocs()

	for b.Loop() {
		_, _, _ = state.Measure(fluidTestCategories())
	}
}
