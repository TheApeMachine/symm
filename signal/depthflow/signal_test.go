package depthflow

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/spf13/viper"
	krakenmarket "github.com/theapemachine/symm/kraken/market"
	"github.com/theapemachine/symm/logic"
	"github.com/theapemachine/symm/market"
)

func setDepthFlowTestConfig() {
	viper.Set("signals.depthflow.measurements_capacity", 4)
}

func seedBooks(
	signal *Signal,
	symbol string,
	base time.Time,
	frames []*krakenmarket.BookUpdate,
) {
	for index, frame := range frames {
		update := *frame
		update.Symbol = symbol
		update.Timestamp = base.Add(time.Duration(index) * time.Millisecond)
		signal.Record(&update)
	}
}

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

func observeRow(symbol string, price, value, volume, pressure float64, eventAt time.Time) {
	row, err := krakenmarket.NewSymbolRow(symbol, price, value, volume, pressure, eventAt)

	if err != nil {
		panic(err)
	}

	if err := crossSection.Observe(row); err != nil {
		panic(err)
	}
}

func TestSignalMeasure(t *testing.T) {
	setDepthFlowTestConfig()
	eventAt := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	measureAt := eventAt.Add(time.Second)

	Convey("Given a bid-heavy book", t, func() {
		useCrossSection(t)

		observeRow("BTC/EUR", 100, 1, 10000, 0.8, eventAt)

		signal := NewSignal(
			"BTC/EUR",
			logic.NewEntity(logic.EntityBook),
		)

		seedBooks(signal, "BTC/EUR", eventAt, []*krakenmarket.BookUpdate{
			{
				Type: "snapshot",
				Bids: []krakenmarket.BookLevel{
					{Price: 99, Qty: 10},
					{Price: 98, Qty: 20},
				},
				Asks: []krakenmarket.BookLevel{
					{Price: 101, Qty: 1},
					{Price: 102, Qty: 1},
				},
			},
			{
				Type: "snapshot",
				Bids: []krakenmarket.BookLevel{
					{Price: 99, Qty: 10},
					{Price: 98, Qty: 20},
				},
				Asks: []krakenmarket.BookLevel{
					{Price: 101, Qty: 1},
					{Price: 102, Qty: 1},
				},
			},
			{
				Type: "snapshot",
				Bids: []krakenmarket.BookLevel{
					{Price: 99, Qty: 12},
					{Price: 98, Qty: 22},
				},
				Asks: []krakenmarket.BookLevel{
					{Price: 101, Qty: 1},
					{Price: 102, Qty: 1},
				},
			},
			{
				Type: "snapshot",
				Bids: []krakenmarket.BookLevel{
					{Price: 99, Qty: 14},
					{Price: 98, Qty: 24},
				},
				Asks: []krakenmarket.BookLevel{
					{Price: 101, Qty: 1},
					{Price: 102, Qty: 1},
				},
			},
		})

		measurement, err := signal.Measure(nil, measureAt)

		Convey("It should classify loaded imbalance", func() {
			So(err, ShouldBeNil)
			So(measurement.Source, ShouldEqual, logic.SourceDepthFlow)
			So(measurement.Category, ShouldNotEqual, logic.CategoryTypeNone)
			So(measurement.Strength, ShouldBeGreaterThan, 0)
			So(measurement.Confidence, ShouldBeGreaterThan, 0)
		})
	})

	Convey("Given deep bid wall with bearish touch", t, func() {
		useCrossSection(t)

		observeRow("ETH/EUR", 50, 1, 10000, -0.5, eventAt)

		signal := NewSignal(
			"ETH/EUR",
			logic.NewEntity(logic.EntityBook),
		)

		seedBooks(signal, "ETH/EUR", eventAt, []*krakenmarket.BookUpdate{
			{
				Type: "snapshot",
				Bids: []krakenmarket.BookLevel{
					{Price: 49, Qty: 1},
					{Price: 48, Qty: 30},
				},
				Asks: []krakenmarket.BookLevel{
					{Price: 51, Qty: 8},
					{Price: 52, Qty: 8},
				},
			},
			{
				Type: "snapshot",
				Bids: []krakenmarket.BookLevel{
					{Price: 49, Qty: 2},
					{Price: 48, Qty: 30},
				},
				Asks: []krakenmarket.BookLevel{
					{Price: 51, Qty: 8},
					{Price: 52, Qty: 8},
				},
			},
			{
				Type: "snapshot",
				Bids: []krakenmarket.BookLevel{
					{Price: 49, Qty: 2},
					{Price: 48, Qty: 30},
				},
				Asks: []krakenmarket.BookLevel{
					{Price: 51, Qty: 8},
					{Price: 52, Qty: 8},
				},
			},
			{
				Type: "snapshot",
				Bids: []krakenmarket.BookLevel{
					{Price: 49, Qty: 2},
					{Price: 48, Qty: 30},
				},
				Asks: []krakenmarket.BookLevel{
					{Price: 51, Qty: 8},
					{Price: 52, Qty: 8},
				},
			},
		})

		measurement, err := signal.Measure(nil, measureAt)

		Convey("It should classify spoof trap", func() {
			So(err, ShouldBeNil)
			So(measurement.Category, ShouldEqual, logic.CategorySpoofTrap)
			So(measurement.Confidence, ShouldBeGreaterThan, 0)
		})
	})

	Convey("Given trade pressure update", t, func() {
		useCrossSection(t)

		signal := NewSignal(
			"SOL/EUR",
			logic.NewEntity(logic.EntityTrade),
		)

		for index, price := range []float64{25, 25.1} {
			signal.Record(&krakenmarket.TradeUpdate{
				Symbol:    "SOL/EUR",
				Side:      "buy",
				Price:     price,
				Qty:       float64(index + 2),
				Timestamp: eventAt.Add(time.Duration(index) * time.Millisecond),
			})
		}

		_, err := signal.Measure(nil, measureAt)

		Convey("It should observe trade pressure while awaiting book", func() {
			So(err, ShouldBeNil)

			pressure, pressureErr := crossSection.Pressure("SOL/EUR")
			So(pressureErr, ShouldBeNil)
			So(pressure, ShouldBeGreaterThan, 0)
		})
	})
}

func TestSignalMeasureBeforeUniverseEntry(t *testing.T) {
	setDepthFlowTestConfig()
	eventAt := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	measureAt := eventAt.Add(time.Second)

	Convey("Given a book update before the symbol enters the cross-section", t, func() {
		useCrossSection(t)

		signal := NewSignal(
			"ZBCN/USD",
			logic.NewEntity(logic.EntityBook),
		)

		seedBooks(signal, "ZBCN/USD", eventAt, []*krakenmarket.BookUpdate{
			{
				Type: "snapshot",
				Bids: []krakenmarket.BookLevel{
					{Price: 99, Qty: 10},
					{Price: 98, Qty: 20},
				},
				Asks: []krakenmarket.BookLevel{
					{Price: 101, Qty: 1},
					{Price: 102, Qty: 1},
				},
			},
			{
				Type: "snapshot",
				Bids: []krakenmarket.BookLevel{
					{Price: 99, Qty: 10},
					{Price: 98, Qty: 20},
				},
				Asks: []krakenmarket.BookLevel{
					{Price: 101, Qty: 1},
					{Price: 102, Qty: 1},
				},
			},
			{
				Type: "snapshot",
				Bids: []krakenmarket.BookLevel{
					{Price: 99, Qty: 12},
					{Price: 98, Qty: 22},
				},
				Asks: []krakenmarket.BookLevel{
					{Price: 101, Qty: 1},
					{Price: 102, Qty: 1},
				},
			},
			{
				Type: "snapshot",
				Bids: []krakenmarket.BookLevel{
					{Price: 99, Qty: 14},
					{Price: 98, Qty: 24},
				},
				Asks: []krakenmarket.BookLevel{
					{Price: 101, Qty: 1},
					{Price: 102, Qty: 1},
				},
			},
		})

		measurement, err := signal.Measure(nil, measureAt)

		Convey("It should measure without halting on missing trade pressure", func() {
			So(err, ShouldBeNil)
			So(measurement.Publishable(), ShouldBeTrue)
			So(measurement.Symbol, ShouldEqual, "ZBCN/USD")
		})
	})
}

func TestSignalMeasureBestEffort(t *testing.T) {
	setDepthFlowTestConfig()

	Convey("Given one book update in the ring", t, func() {
		signal := NewSignal("BTC/EUR", logic.NewEntity(logic.EntityBook))
		base := time.Date(2026, 5, 30, 12, 0, 0, 0, time.UTC)

		signal.Record(&krakenmarket.BookUpdate{
			Symbol:    "BTC/EUR",
			Timestamp: base,
			Bids:      []krakenmarket.BookLevel{{Price: 99, Qty: 1}},
			Asks:      []krakenmarket.BookLevel{{Price: 101, Qty: 1}},
		})

		measurement, err := signal.Measure(nil, base.Add(time.Second))

		Convey("It should return an empty measurement without enough ring history", func() {
			So(err, ShouldBeNil)
			So(measurement.Publishable(), ShouldBeFalse)
		})
	})
}
