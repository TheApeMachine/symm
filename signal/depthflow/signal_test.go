package depthflow

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	krakenmarket "github.com/theapemachine/symm/kraken/market"
	"github.com/theapemachine/symm/logic"
	"github.com/theapemachine/symm/market"
)

func initCrossSection(cfg *market.CrossSectionConfig) {
	section, err := market.NewCrossSection(cfg)
	if err != nil {
		panic(err)
	}

	crossSection = section
}

func useCrossSection(t *testing.T) {
	t.Helper()

	initCrossSection(&market.CrossSectionConfig{
		MatchWindow: time.Minute,
		ReturnCap:   64,
		MinBars:     8,
		BreadthHist: 64,
	})
}

func TestSignalMeasure(t *testing.T) {
	Convey("Given a bid-heavy book", t, func() {
		useCrossSection(t)

		crossSection.Observe(&krakenmarket.Symbol{Name: "BTC/EUR", Pressure: 0.8})

		signal := NewSignal(
			"BTC/EUR",
			logic.NewEntity(logic.EntityBook),
		)

		signal.Record(&krakenmarket.BookUpdate{
			Symbol: "BTC/EUR",
			Type:   "snapshot",
			Bids: []krakenmarket.BookLevel{
				{Price: 99, Qty: 10},
				{Price: 98, Qty: 20},
			},
			Asks: []krakenmarket.BookLevel{
				{Price: 101, Qty: 1},
				{Price: 102, Qty: 1},
			},
		})

		measurement, err := signal.Measure(nil, time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC))

		Convey("It should classify loaded imbalance", func() {
			So(err, ShouldBeNil)
			So(measurement.Source, ShouldEqual, logic.SourceDepthFlow)
			So(measurement.Category, ShouldEqual, logic.CategoryLoadedImbalance)
			So(measurement.Strength, ShouldBeGreaterThan, 0)
		})
	})

	Convey("Given deep bid wall with bearish touch", t, func() {
		useCrossSection(t)

		signal := NewSignal(
			"ETH/EUR",
			logic.NewEntity(logic.EntityBook),
		)

		signal.Record(&krakenmarket.BookUpdate{
			Symbol: "ETH/EUR",
			Type:   "snapshot",
			Bids: []krakenmarket.BookLevel{
				{Price: 49, Qty: 1},
				{Price: 48, Qty: 30},
			},
			Asks: []krakenmarket.BookLevel{
				{Price: 51, Qty: 8},
				{Price: 52, Qty: 8},
			},
		})

		measurement, err := signal.Measure(nil, time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC))

		Convey("It should classify spoof trap", func() {
			So(err, ShouldBeNil)
			So(measurement.Category, ShouldEqual, logic.CategorySpoofTrap)
		})
	})

	Convey("Given trade pressure update", t, func() {
		useCrossSection(t)

		signal := NewSignal(
			"SOL/EUR",
			logic.NewEntity(logic.EntityTrade),
		)

		signal.Record(&krakenmarket.TradeUpdate{
			Symbol: "SOL/EUR",
			Side:   "buy",
			Price:  25,
			Qty:    3,
		})

		signal.Record(&krakenmarket.TradeUpdate{
			Symbol: "SOL/EUR",
			Side:   "buy",
			Price:  25.1,
			Qty:    2,
		})

		measurement, err := signal.Measure(nil, time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC))

		Convey("It should publish trade pressure without category", func() {
			So(err, ShouldBeNil)
			So(measurement.Symbol, ShouldEqual, "SOL/EUR")

			pressure, pressureErr := crossSection.Pressure("SOL/EUR")
			So(pressureErr, ShouldBeNil)
			So(pressure, ShouldBeGreaterThan, 0)
		})
	})
}
