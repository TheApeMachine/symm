package types

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/nomagique/transport"
	nmtypes "github.com/theapemachine/symm/nomagique/types"
	wire "github.com/theapemachine/symm/telemetry/generated/telemetry"
)

func TestSymbolNewSymbol(t *testing.T) {
	Convey("Given a symbol name", t, func() {
		symbol := NewSymbol("BTC/USD")

		Convey("It should initialize empty symbol stream state", func() {
			So(symbol.Symbol, ShouldEqual, "BTC/USD")
			So(symbol.Status, ShouldEqual, READY)
			So(symbol.Measurements, ShouldNotBeNil)
			So(symbol.Positions, ShouldNotBeNil)
		})
	})
}

func TestSymbolAppendMeasurement(t *testing.T) {
	Convey("Given one raw-only measurement appended to a symbol", t, func() {
		previousFocus := Focus()
		SetFocus("BTC/USD")
		Reset(func() { SetFocus(previousFocus) })

		dashboard := transport.NewConsumer[*UIFrame]("dashboard", func() {})
		ui := transport.NewMapReduce[*UIFrame](
			[]*transport.Consumer[*UIFrame]{dashboard}, nil, nil,
		)
		symbol := NewSymbol("BTC/USD", ui)
		symbol.Tick = 42
		measurement := nmtypes.NewMeasurement("first", string(SourceHawkes), 0, 0)

		symbol.AppendMeasurement(measurement)

		Convey("It should expose the row to every consuming solver", func() {
			So(measurement.Symbol, ShouldEqual, "BTC/USD")
			So(measurement.Tick, ShouldEqual, 42)

			for _, consumer := range []*transport.Consumer[*nmtypes.Measurement]{
				symbol.MeasurementConsumers[MeasurementConsumerCategory],
				symbol.MeasurementConsumers[MeasurementConsumerGraph],
				symbol.MeasurementConsumers[MeasurementConsumerManifold],
			} {
				rows := make([]*nmtypes.Measurement, 0)

				for row := range symbol.MarketMeasurements(consumer) {
					rows = append(rows, row)
				}

				So(len(rows), ShouldBeGreaterThanOrEqualTo, 1)

				if len(rows) > 0 {
					So(rows[0].ID, ShouldEqual, "first")
				}
			}
		})

		Convey("It should publish the measurement to the dashboard transport", func() {
			frame, found := ui.Pop(dashboard)
			So(found, ShouldBeTrue)
			So(frame.Type, ShouldEqual, wire.FrameMeasurementsFrame)
			rows := frame.Value.(*wire.MeasurementsFrameT).Rows
			So(rows, ShouldHaveLength, 1)
			So(rows[0].Id, ShouldEqual, "first")
		})
	})

	Convey("Given many measurements appended for audit and live solvers", t, func() {
		symbol := NewSymbol("BTC/USD")

		for index := range 8 {
			measurement := nmtypes.NewMeasurement(
				"row-"+string(rune('a'+index)),
				string(SourceHawkes),
				0,
				0,
			)
			symbol.AppendMeasurement(measurement)
		}

		Convey("It should keep the audit cursor at the newest row", func() {
			audit := symbol.MeasurementConsumers[MeasurementConsumerAudit]
			So(symbol.Measurements.Length(audit), ShouldEqual, uint64(1))
			row, found := symbol.Measurements.Pop(audit)
			So(found, ShouldBeTrue)
			So(row.ID, ShouldEqual, "row-h")
		})

		Convey("It should retain the full FIFO for live solvers", func() {
			category := symbol.MeasurementConsumers[MeasurementConsumerCategory]
			So(symbol.Measurements.Length(category), ShouldEqual, uint64(8))
		})
	})

	Convey("Given a measurement identified as a different symbol", t, func() {
		symbol := NewSymbol("BTC/USD")
		measurement := nmtypes.NewMeasurement("wrong", string(SourceHawkes), 0, 0)
		measurement.Symbol = "ETH/USD"

		Convey("It should reject corrupted provenance", func() {
			So(func() { symbol.AppendMeasurement(measurement) }, ShouldPanic)
		})
	})

	Convey("Given a measurement appended outside the dashboard focus", t, func() {
		previousFocus := Focus()
		SetFocus("BTC/USD")
		Reset(func() { SetFocus(previousFocus) })

		dashboard := transport.NewConsumer[*UIFrame]("dashboard", func() {})
		ui := transport.NewMapReduce[*UIFrame](
			[]*transport.Consumer[*UIFrame]{dashboard}, nil, nil,
		)
		symbol := NewSymbol("ETH/USD", ui)
		measurement := nmtypes.NewMeasurement("unfocused", string(SourceHawkes), 0, 0)

		symbol.AppendMeasurement(measurement)

		Convey("It should retain the measurement without publishing a UI frame", func() {
			row, found := symbol.Measurements.Pop(
				symbol.MeasurementConsumers[MeasurementConsumerAudit],
			)
			So(found, ShouldBeTrue)
			So(row.ID, ShouldEqual, "unfocused")

			_, found = ui.Pop(dashboard)
			So(found, ShouldBeFalse)
		})
	})
}

func TestSymbolAppendTicker(t *testing.T) {
	Convey("Given one ticker appended to a symbol", t, func() {
		symbol := NewSymbol("BTC/USD")
		ticker := kraken.TickerData{Symbol: "BTC/USD"}

		symbol.AppendTicker(ticker)

		Convey("It should queue the ticker", func() {
			liquidity := symbol.TickerConsumers[TickerConsumerLiquidity]
			sentiment := symbol.TickerConsumers[TickerConsumerSentiment]
			So(symbol.HasTickersFor(liquidity), ShouldBeTrue)
			rows := make([]kraken.TickerData, 0)

			for row := range symbol.MarketTickers(liquidity) {
				rows = append(rows, row)
			}

			So(rows, ShouldResemble, []kraken.TickerData{ticker})
			So(symbol.HasTickersFor(liquidity), ShouldBeFalse)
			So(symbol.HasTickersFor(sentiment), ShouldBeTrue)
		})
	})
}

func TestSymbolAppendTrade(t *testing.T) {
	Convey("Given one trade appended to a symbol", t, func() {
		symbol := NewSymbol("BTC/USD")
		trade := kraken.TradeData{Symbol: "BTC/USD"}

		symbol.AppendTrade(trade)

		Convey("It should queue the trade", func() {
			cvd := symbol.TradeConsumers[TradeConsumerCVD]
			hawkes := symbol.TradeConsumers[TradeConsumerHawkes]
			So(symbol.HasTradesFor(cvd), ShouldBeTrue)
			So(symbol.HasTradesFor(hawkes), ShouldBeTrue)
			rows := make([]kraken.TradeData, 0)

			for row := range symbol.MarketTrades(cvd) {
				rows = append(rows, row)
			}

			So(rows, ShouldResemble, []kraken.TradeData{trade})
			So(symbol.HasTradesFor(cvd), ShouldBeFalse)
			So(symbol.HasTradesFor(hawkes), ShouldBeTrue)
		})
	})
}

func TestSymbolAppendLevel3(t *testing.T) {
	Convey("Given one Level 3 frame appended to a symbol", t, func() {
		symbol := NewSymbol("BTC/USD")
		level3 := kraken.Level3Data{Symbol: "BTC/USD"}

		symbol.AppendLevel3(level3)

		Convey("It should queue the frame only for Level 3 consumers", func() {
			depthFlow := symbol.Level3Consumers[Level3ConsumerDepthFlow]
			toxicity := symbol.Level3Consumers[Level3ConsumerToxicity]
			So(symbol.HasLevel3For(depthFlow), ShouldBeTrue)
			So(symbol.HasLevel3For(toxicity), ShouldBeTrue)
			rows := make([]kraken.Level3Data, 0)

			for row := range symbol.MarketLevel3(depthFlow) {
				rows = append(rows, row)
			}

			So(rows, ShouldResemble, []kraken.Level3Data{level3})
		})
	})

	Convey("Given many Level 3 frames appended to a symbol", t, func() {
		symbol := NewSymbol("BTC/USD")

		for index := range 4 {
			symbol.AppendLevel3(kraken.Level3Data{
				Symbol:   "BTC/USD",
				Checksum: uint32(index + 1),
			})
		}

		Convey("It should keep one frame for geometry readers and every frame for toxicity", func() {
			depthFlow := symbol.Level3Consumers[Level3ConsumerDepthFlow]
			pumpDump := symbol.Level3Consumers[Level3ConsumerPumpDump]
			toxicity := symbol.Level3Consumers[Level3ConsumerToxicity]
			So(symbol.level3.Length(depthFlow), ShouldEqual, uint64(1))
			So(symbol.level3.Length(pumpDump), ShouldEqual, uint64(1))
			So(symbol.level3.Length(toxicity), ShouldEqual, uint64(4))
		})
	})
}

func TestSymbolQueueDepths(t *testing.T) {
	Convey("Given measurements appended to one symbol", t, func() {
		symbol := NewSymbol("BTC/USD")

		for range 5 {
			symbol.AppendMeasurement(
				nmtypes.NewMeasurement("row", string(SourceHawkes), 0, 0),
			)
		}

		Convey("It should report live solver pressure without the audit cursor", func() {
			depths := symbol.QueueDepths()
			So(depths["measurements"], ShouldEqual, uint64(15))
			So(symbol.Measurements.Length(
				symbol.MeasurementConsumers[MeasurementConsumerAudit],
			), ShouldEqual, uint64(1))
		})
	})
}

func BenchmarkSymbolAppendMeasurement(b *testing.B) {
	previousFocus := Focus()
	SetFocus("BTC/USD")
	b.Cleanup(func() { SetFocus(previousFocus) })

	dashboard := transport.NewConsumer[*UIFrame]("dashboard", func() {})
	ui := transport.NewMapReduce[*UIFrame](
		[]*transport.Consumer[*UIFrame]{dashboard}, nil, nil,
	)

	b.Run("focused", func(b *testing.B) {
		symbol := NewSymbol("BTC/USD", ui)
		consumers := symbol.MeasurementConsumers
		measurement := nmtypes.NewMeasurement("benchmark", string(SourceHawkes), 0, 0)
		b.ReportAllocs()

		for range b.N {
			symbol.AppendMeasurement(measurement)
			ui.Pop(dashboard)

			for _, consumer := range consumers {
				symbol.Measurements.Pop(consumer)
			}
		}
	})

	b.Run("unfocused", func(b *testing.B) {
		symbol := NewSymbol("ETH/USD", ui)
		consumers := symbol.MeasurementConsumers
		measurement := nmtypes.NewMeasurement("benchmark", string(SourceHawkes), 0, 0)
		b.ReportAllocs()

		for range b.N {
			symbol.AppendMeasurement(measurement)

			for _, consumer := range consumers {
				symbol.Measurements.Pop(consumer)
			}
		}
	})
}
