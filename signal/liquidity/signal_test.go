package liquidity

import (
	"context"
	"testing"
	"time"

	krakendecimal "github.com/krakenfx/api-go/v2/pkg/decimal"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/kraken/websocket"
	"github.com/theapemachine/symm/tests"
	"github.com/theapemachine/symm/tests/conditions"
	"github.com/theapemachine/symm/types"
)

func measureField(measurements []*types.Measurement, symbol string, metric types.MetricType) (*types.Measurement, bool) {
	for index := len(measurements) - 1; index >= 0; index-- {
		measurement := measurements[index]

		if measurement.Symbol == symbol && measurement.Metric == metric {
			return measurement, true
		}
	}

	return nil, false
}

func liquidityRow(
	symbol string, bid, ask, bidQty, askQty, volume, vwap float64,
) kraken.TickerData {
	return kraken.TickerData{
		Symbol:    symbol,
		Bid:       krakendecimal.NewFromFloat64(bid),
		BidQty:    bidQty,
		Ask:       krakendecimal.NewFromFloat64(ask),
		AskQty:    askQty,
		Last:      krakendecimal.NewFromFloat64((bid + ask) / 2),
		Volume:    volume,
		Vwap:      vwap,
		Timestamp: time.Now(),
	}
}

// liquidityFrame builds a MarketFrame carrying the given ticker rows and a
// cross-section populated from them, matching how production feeds Calculate.
func liquidityFrame(rows ...kraken.TickerData) *types.MarketFrame {
	crossSection := types.NewCrossSection()
	crossSection.Measure(rows)

	return &types.MarketFrame{
		Tickers:      rows,
		CrossSection: crossSection,
	}
}

func measure(signal *Signal, rows ...kraken.TickerData) []*types.Measurement {
	measurements, err := signal.Calculate(liquidityFrame(rows...))
	So(err, ShouldBeNil)

	return measurements
}

func TestSignal_MeasureRequiresTwoExecutablePeers(t *testing.T) {
	Convey("Given a cross-section with only one observed symbol", t, func() {
		signal := &Signal{}

		result := measure(signal,
			liquidityRow("BTC/USD", 999, 1001, 2, 2, 1, 1000),
		)

		Convey("Then it emits provisional depth without inventing a peer median", func() {
			depth, ok := measureField(result, "BTC/USD", types.MetricExecutableTouchDepth)
			So(ok, ShouldBeTrue)
			So(depth.Validity.State, ShouldEqual, types.ValidityProvisional)
			So(depth.Validity.Reason, ShouldContainSubstring, "peer executable-depth median")

			_, hasRelative := measureField(result, "BTC/USD", types.MetricRelativeTouchDepth)
			So(hasRelative, ShouldBeFalse)
		})
	})
}

func TestSignal_MeasureUsesExecutableValueNotRawVolume(t *testing.T) {
	Convey("Given two penny-priced peers with huge raw volume but tiny quote notional", t, func() {
		signal := &Signal{}

		result := measure(signal,
			liquidityRow("BTC/USD", 999, 1001, 2, 2, 1, 1000),
			liquidityRow("PENNY1/USD", 0.0001, 0.0001, 1_000_000, 1_000_000, 1_000_000, 0.0001),
			liquidityRow("PENNY2/USD", 0.0001, 0.0001, 1_000_000, 1_000_000, 1_000_000, 0.0001),
		)

		Convey("Then current executable depth, not raw units, determines scarcity", func() {
			relative, ok := measureField(result, "BTC/USD", types.MetricRelativeTouchDepth)
			So(ok, ShouldBeTrue)
			So(relative.Raw, ShouldBeGreaterThan, 1)
			So(relative.Subject, ShouldEqual, types.SubjectPeerLiquidity)
			So(relative.Maturity, ShouldBeGreaterThan, 0)

			strength, ok := measureField(result, "BTC/USD", types.MetricScarcityScore)
			So(ok, ShouldBeTrue)
			So(strength.Raw, ShouldEqual, 0)
		})
	})
}

func TestSignal_MeasureAtPeerMedianIsBalanced(t *testing.T) {
	Convey("Given a subject whose notional and depth match its peers exactly", t, func() {
		signal := &Signal{}

		result := measure(signal,
			liquidityRow("BTC/USD", 99, 101, 5, 5, 100, 100),
			liquidityRow("ETH/USD", 99, 101, 5, 5, 100, 100),
			liquidityRow("SOL/USD", 99, 101, 5, 5, 100, 100),
		)

		Convey("Then relative touch depth is one and scarcity is zero", func() {
			relative, ok := measureField(result, "BTC/USD", types.MetricRelativeTouchDepth)
			So(ok, ShouldBeTrue)
			So(relative.Raw, ShouldAlmostEqual, 1, 1e-9)

			strength, ok := measureField(result, "BTC/USD", types.MetricScarcityScore)
			So(ok, ShouldBeTrue)
			So(strength.Raw, ShouldEqual, 0)
		})
	})
}

func TestSignal_MeasureDoesNotMixTurnoverIntoTouchDepth(t *testing.T) {
	Convey("Given high reported turnover but below-median executable touch depth", t, func() {
		signal := &Signal{}

		result := measure(signal,
			liquidityRow("BTC/USD", 99, 101, 0.5, 0.5, 1_000_000, 100),
			liquidityRow("ETH/USD", 99, 101, 1, 1, 100, 100),
			liquidityRow("SOL/USD", 99, 101, 1, 1, 100, 100),
		)

		Convey("Then turnover cannot inflate the current depth ratio", func() {
			relative, ok := measureField(result, "BTC/USD", types.MetricRelativeTouchDepth)
			So(ok, ShouldBeTrue)
			So(relative.Raw, ShouldAlmostEqual, 0.5, 1e-9)

			strength, ok := measureField(result, "BTC/USD", types.MetricScarcityScore)
			So(ok, ShouldBeTrue)
			So(strength.Raw, ShouldAlmostEqual, 0.5, 1e-9)
		})
	})
}

func TestSignal_MeasureDoesNotRequireReportedTurnover(t *testing.T) {
	Convey("Given executable peers without reported turnover", t, func() {
		signal := &Signal{}

		result := measure(signal,
			liquidityRow("BTC/USD", 99, 101, 0.5, 0.5, 0, 0),
			liquidityRow("ETH/USD", 99, 101, 1, 1, 0, 0),
			liquidityRow("SOL/USD", 99, 101, 1, 1, 0, 0),
		)

		Convey("Then touch scarcity remains measurable without invented turnover", func() {
			strength, ok := measureField(result, "BTC/USD", types.MetricScarcityScore)
			So(ok, ShouldBeTrue)
			So(strength.Raw, ShouldAlmostEqual, 0.5, 1e-9)

			_, hasNotional := measureField(result, "BTC/USD", types.MetricReportedVolumeNotional)
			So(hasNotional, ShouldBeFalse)
		})
	})
}

func TestSignal_MeasureSkipsNonExecutableSubject(t *testing.T) {
	Convey("Given a subject with a one-sided quote among executable peers", t, func() {
		signal := &Signal{}

		result := measure(signal,
			kraken.TickerData{
				Symbol:    "BTC/USD",
				Bid:       krakendecimal.NewFromFloat64(0),
				BidQty:    0,
				Ask:       krakendecimal.NewFromFloat64(101),
				AskQty:    5,
				Last:      krakendecimal.NewFromFloat64(101),
				Volume:    100,
				Vwap:      100,
				Timestamp: time.Now(),
			},
			liquidityRow("ETH/USD", 99, 101, 5, 5, 100, 100),
			liquidityRow("SOL/USD", 99, 101, 5, 5, 100, 100),
		)

		Convey("Then it emits nothing for the unexecutable subject", func() {
			_, hasSubject := measureField(result, "BTC/USD", types.MetricRelativeTouchDepth)
			So(hasSubject, ShouldBeFalse)
		})
	})
}

func sessionSignals(
	ctx context.Context,
	api *websocket.API,
	_ *broker.Instrument,
	channel chan []byte,
) []types.Signal {
	return []types.Signal{NewSignal(ctx, api, channel)}
}

/*
TestSignal_MeasureFromMarket proves liquidity on the mock Conn Session path:
a thin-book tape must emit scarcity and relative-touch evidence, and scarcity
must exceed a sector-lift (herd) peer — not a relative-only smoke check.
*/
func TestSignal_MeasureFromMarket(t *testing.T) {
	symbol := conditions.Subject()
	options := tests.SessionOptions{Signals: sessionSignals}

	thinFamily := []tests.SourceClaim{
		{Source: types.SourceLiquidity, Metric: types.MetricScarcityScore, Symbol: symbol, Bound: tests.BoundPositive},
		{Source: types.SourceLiquidity, Metric: types.MetricRelativeTouchDepth, Symbol: symbol, Bound: tests.BoundPresent},
	}

	t.Run("tape_thin_book", func(t *testing.T) {
		tests.PlayMarketClaims(t, options, conditions.TapeThinBook().Frames(), thinFamily...)
	})

	t.Run("thin_exceeds_herd_scarcity", func(t *testing.T) {
		herd := tests.PlayMarketClaims(t, options, conditions.TapeSectorLift().Frames())
		thin := tests.PlayMarketClaims(
			t, options, conditions.TapeThinBook().Frames(), thinFamily...,
		)
		tests.RequireSourceExceeds(
			t, thin, herd,
			types.SourceLiquidity, symbol, types.MetricScarcityScore,
		)
	})
}

func BenchmarkSignal_MeasureFromMarket(benchmark *testing.B) {
	session, err := tests.NewSession(context.Background(), benchmark, tests.SessionOptions{
		Signals: sessionSignals,
	})

	if err != nil {
		benchmark.Fatal(err)
	}

	frames := conditions.TapeThinBook().Frames()
	benchmark.ReportAllocs()

	for benchmark.Loop() {
		if _, err := session.Play(frames); err != nil {
			benchmark.Fatal(err)
		}
	}
}

func BenchmarkSignal_Measure(benchmark *testing.B) {
	signal := &Signal{}
	frame := liquidityFrame(
		liquidityRow("BTC/USD", 999, 1001, 2, 2, 1, 1000),
		liquidityRow("ETH/USD", 99, 101, 5, 5, 100, 100),
		liquidityRow("SOL/USD", 99, 101, 5, 5, 100, 100),
	)

	benchmark.ReportAllocs()

	for benchmark.Loop() {
		_, _ = signal.Calculate(frame)
	}
}
