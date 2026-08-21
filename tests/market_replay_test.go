package tests

import (
	"bytes"
	"os"
	"testing"
	"time"

	spotbook "github.com/krakenfx/api-go/v2/pkg/book"
	"github.com/krakenfx/api-go/v2/pkg/spot"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/spf13/viper"
	"github.com/theapemachine/symm/backtest"
	testtypes "github.com/theapemachine/symm/tests/types"
)

func TestMarketReplayFrame(t *testing.T) {
	previousDepth, depthWasSet := viper.GetInt("market.l3_depth"),
		viper.IsSet("market.l3_depth")
	viper.Set("market.l3_depth", 10)
	defer func() {
		if depthWasSet {
			viper.Set("market.l3_depth", previousDepth)
			return
		}

		viper.Set("market.l3_depth", nil)
	}()

	Convey("Given one ticker, trade, and resident Level 3 capture frame", t, func() {
		symbol := testtypes.NewSymbol("MATIC/USD", 0.5637, 13)
		symbol.PriceIncrement = 0.0001
		symbol.PricePrecision = 4
		symbol.QuantityPrecision = 8
		symbol.BookDepthLevels = 10
		config := testtypes.NewScenarioConfig([]*testtypes.Symbol{symbol})
		config.InitialBalances = map[string]float64{"USD": 10}
		config.Execution.DepthLevels = 10
		market, err := NewMarketWithScenario(t.Context(), config)
		So(err, ShouldBeNil)
		defer market.Close()
		market.private.SubL3([]string{symbol.Pair})
		market.WithAutoFill(config.Execution)
		_, err = market.Private.transport.addOrder(spot.AddOrderRequest{
			ClOrdId: "resident-level3", OrderType: "market", Type: "buy",
			Volume: "1", Pair: symbol.Pair,
		})
		So(err, ShouldBeNil)
		tickerPayload, err := os.ReadFile("fixtures/ticker/fixtures/snapshot.json")
		So(err, ShouldBeNil)
		tradePayload, err := os.ReadFile("fixtures/trade/fixtures/update.json")
		So(err, ShouldBeNil)
		level3Payload, err := os.ReadFile("fixtures/level3/fixtures/snapshot.json")
		So(err, ShouldBeNil)
		// The fixture venue declares four price and eight quantity decimals.
		// This is the CRC32 of those exact resident fixed-point values.
		level3Payload = bytes.Replace(
			level3Payload,
			[]byte(`"checksum":2841398499`),
			[]byte(`"checksum":4186435250`),
			1,
		)
		So(bytes.Contains(level3Payload, []byte(`"checksum":4186435250`)),
			ShouldBeTrue)
		arrival := time.Date(2026, time.August, 21, 8, 0, 0, 0, time.UTC)

		err = market.ReplayFrame(backtest.Frame{
			Endpoint: "public", Payload: tickerPayload, ReceivedAt: arrival,
		})
		So(err, ShouldBeNil)
		report := market.Report()
		sample, found := market.LastSample(symbol.Pair)
		observationCalls := 0
		settledResidentBookBeforeExecution := false
		market.replayObservation = func() error {
			observationCalls++
			market.private.Book(symbol.Pair, func(book *spotbook.Book) {
				if !book.L3Checksum("4186435250").Match {
					return
				}

				report = market.Report()
				settledResidentBookBeforeExecution =
					report.Mechanics.Filled == 0 &&
						len(market.Private.transport.pending) == 1
			})

			return nil
		}

		Convey("Ticker state should advance and publish exactly once", func() {
			So(found, ShouldBeTrue)
			So(sample.Last, ShouldEqual, 0.10035)
			So(sample.Ask, ShouldEqual, 0.10036)
			So(report.Tick, ShouldEqual, uint64(1))
			So(capturedPayloadCount(report.PublicTransport, tickerPayload),
				ShouldEqual, 1)
			So(capturedPayloadCount(report.Level3Transport, level3Payload),
				ShouldEqual, 0)
			So(report.Mechanics.Submitted, ShouldEqual, 0)
			So(report.Mechanics.Filled, ShouldEqual, 0)
			So(market.previous, ShouldNotContainKey, symbol.Pair)
			So(market.Private.transport.pending, ShouldHaveLength, 1)
		})

		err = market.ReplayFrame(backtest.Frame{
			Endpoint: "public", Payload: tradePayload,
			ReceivedAt: arrival.Add(time.Millisecond),
		})
		So(err, ShouldBeNil)
		report = market.Report()
		sample, found = market.LastSample(symbol.Pair)

		Convey("Trade state should advance and publish exactly once without execution", func() {
			So(found, ShouldBeTrue)
			So(sample.Last, ShouldEqual, 0.5117)
			So(sample.StepVolume, ShouldEqual, 40.0)
			So(sample.AggressorSide, ShouldEqual, "sell")
			So(report.Tick, ShouldEqual, uint64(1))
			So(capturedPayloadCount(report.PublicTransport, tickerPayload),
				ShouldEqual, 1)
			So(capturedPayloadCount(report.PublicTransport, tradePayload),
				ShouldEqual, 1)
			So(capturedPayloadCount(report.Level3Transport, level3Payload),
				ShouldEqual, 0)
			So(report.Mechanics.Submitted, ShouldEqual, 0)
			So(report.Mechanics.Filled, ShouldEqual, 0)
			So(market.Private.transport.pending, ShouldHaveLength, 1)
			So(observationCalls, ShouldEqual, 1)
		})

		err = market.ReplayFrame(backtest.Frame{
			Endpoint: "level3", Payload: level3Payload,
			ReceivedAt: arrival.Add(2 * time.Millisecond),
		})
		So(err, ShouldBeNil)
		report = market.Report()
		bookFound := false

		market.private.Book(symbol.Pair, func(book *spotbook.Book) {
			bookFound = true
			So(book.BestBid().Price.Float64(), ShouldEqual, 0.5634)
			So(book.BestAsk().Price.Float64(), ShouldEqual, 0.564)
			So(book.L3Checksum("4186435250").Match, ShouldBeTrue)
		})

		Convey("Execution should happen once from the checksum-valid resident book", func() {
			So(bookFound, ShouldBeTrue)
			So(observationCalls, ShouldEqual, 2)
			So(settledResidentBookBeforeExecution, ShouldBeTrue)
			So(capturedPayloadCount(report.PublicTransport, tickerPayload),
				ShouldEqual, 1)
			So(capturedPayloadCount(report.PublicTransport, tradePayload),
				ShouldEqual, 1)
			So(capturedPayloadCount(report.Level3Transport, level3Payload),
				ShouldEqual, 1)
			So(report.Mechanics.Submitted, ShouldEqual, 1)
			So(report.Mechanics.Filled, ShouldEqual, 1)
			So(report.Economics.ExecutedQuantity, ShouldEqual, 1.0)
			So(market.Private.transport.pending, ShouldHaveLength, 0)
			So(market.execution.orders, ShouldHaveLength, 1)
			So(market.execution.orders[0].cumulativeCost, ShouldAlmostEqual, 0.564)
		})
	})
}

func capturedPayloadCount(report TransportReport, payload []byte) int {
	count := 0

	for _, frame := range report.Frames {
		if frame.Generated == string(payload) {
			count++
		}
	}

	return count
}

func BenchmarkMarketReplayFrame(b *testing.B) {
	symbol := testtypes.NewSymbol("MATIC/USD", 0.5637, 13)
	config := testtypes.NewScenarioConfig([]*testtypes.Symbol{symbol})
	market, err := NewMarketWithScenario(b.Context(), config)

	if err != nil {
		b.Fatal(err)
	}

	defer market.Close()
	payload, err := os.ReadFile("fixtures/ticker/fixtures/snapshot.json")

	if err != nil {
		b.Fatal(err)
	}

	frame := backtest.Frame{
		Endpoint: "public", Payload: payload,
		ReceivedAt: time.Date(2026, time.August, 21, 8, 0, 0, 0, time.UTC),
	}
	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		if err := market.ReplayFrame(frame); err != nil {
			b.Fatal(err)
		}

		market.Public.faults.mu.Lock()
		market.Public.faults.report.Frames = market.Public.faults.report.Frames[:0]
		market.Public.faults.mu.Unlock()
	}
}
