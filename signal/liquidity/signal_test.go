package liquidity

import (
	"context"
	"testing"
	"time"

	krakendecimal "github.com/krakenfx/api-go/v2/pkg/decimal"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/kraken"
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

func TestSignal_MeasureRequiresTwoExecutablePeers(testingTB *testing.T) {
	Convey("Given a cross-section with only one observed symbol", testingTB, func() {
		signal := &Signal{
			ctx: context.Background(),
			ticker: &Ticker{
				cache: []kraken.TickerData{
					liquidityRow("BTC/USD", 999, 1001, 2, 2, 1, 1000),
				},
			},
		}

		result := signal.Measure(types.NewThesis(nil))

		Convey("Then it emits nothing rather than dividing by an unsupported median", func() {
			So(result.Measurements, ShouldBeEmpty)
		})
	})
}

func TestSignal_MeasureUsesExecutableValueNotRawVolume(testingTB *testing.T) {
	Convey("Given two penny-priced peers with huge raw volume but tiny quote notional", testingTB, func() {
		signal := &Signal{
			ctx: context.Background(),
			ticker: &Ticker{
				cache: []kraken.TickerData{
					liquidityRow("BTC/USD", 999, 1001, 2, 2, 1, 1000),
					liquidityRow("PENNY1/USD", 0.0001, 0.0001, 1_000_000, 1_000_000, 1_000_000, 0.0001),
					liquidityRow("PENNY2/USD", 0.0001, 0.0001, 1_000_000, 1_000_000, 1_000_000, 0.0001),
				},
			},
		}

		result := signal.Measure(types.NewThesis(nil))

		Convey("Then it scores the subject as liquid, not scarce", func() {
			rvol, ok := measureField(result.Measurements, "BTC/USD", types.MetricRVOL)
			So(ok, ShouldBeTrue)
			So(rvol.Raw, ShouldBeGreaterThan, 1)
			So(rvol.Subject, ShouldEqual, types.SubjectPeerLiquidity)
			So(rvol.Maturity, ShouldBeGreaterThan, 0)

			depthScore, ok := measureField(result.Measurements, "BTC/USD", types.MetricDepthScore)
			So(ok, ShouldBeTrue)
			So(depthScore.Raw, ShouldBeGreaterThan, 0)

			scarcity, ok := measureField(result.Measurements, "BTC/USD", types.MetricScarcityScore)
			So(ok, ShouldBeTrue)
			So(scarcity.Raw, ShouldEqual, 0)
		})
	})
}

func TestSignal_MeasureAtPeerMedianIsBalanced(testingTB *testing.T) {
	Convey("Given a subject whose notional and depth match its peers exactly", testingTB, func() {
		signal := &Signal{
			ctx: context.Background(),
			ticker: &Ticker{
				cache: []kraken.TickerData{
					liquidityRow("BTC/USD", 99, 101, 5, 5, 100, 100),
					liquidityRow("ETH/USD", 99, 101, 5, 5, 100, 100),
					liquidityRow("SOL/USD", 99, 101, 5, 5, 100, 100),
				},
			},
		}

		result := signal.Measure(types.NewThesis(nil))

		Convey("Then relative value is one and neither scarcity nor depth dominates", func() {
			rvol, ok := measureField(result.Measurements, "BTC/USD", types.MetricRVOL)
			So(ok, ShouldBeTrue)
			So(rvol.Raw, ShouldAlmostEqual, 1, 1e-9)

			scarcity, ok := measureField(result.Measurements, "BTC/USD", types.MetricScarcityScore)
			So(ok, ShouldBeTrue)
			So(scarcity.Raw, ShouldEqual, 0)

			depthScore, ok := measureField(result.Measurements, "BTC/USD", types.MetricDepthScore)
			So(ok, ShouldBeTrue)
			So(depthScore.Raw, ShouldEqual, 0)

			peerBalance, ok := measureField(result.Measurements, "BTC/USD", types.MetricPeerBalanceScore)
			So(ok, ShouldBeTrue)
			So(peerBalance.Raw, ShouldEqual, 1)
		})
	})
}

func TestSignal_MeasureSkipsNonExecutableSubject(testingTB *testing.T) {
	Convey("Given a subject with a one-sided quote among executable peers", testingTB, func() {
		signal := &Signal{
			ctx: context.Background(),
			ticker: &Ticker{
				cache: []kraken.TickerData{
					{
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
				},
			},
		}

		result := signal.Measure(types.NewThesis(nil))

		Convey("Then it emits nothing for the unexecutable subject", func() {
			_, hasSubject := measureField(result.Measurements, "BTC/USD", types.MetricRVOL)
			So(hasSubject, ShouldBeFalse)
		})
	})
}

func BenchmarkSignal_Measure(benchmark *testing.B) {
	signal := &Signal{
		ctx: context.Background(),
		ticker: &Ticker{
			cache: []kraken.TickerData{
				liquidityRow("BTC/USD", 999, 1001, 2, 2, 1, 1000),
				liquidityRow("ETH/USD", 99, 101, 5, 5, 100, 100),
				liquidityRow("SOL/USD", 99, 101, 5, 5, 100, 100),
			},
		},
	}

	benchmark.ReportAllocs()

	for benchmark.Loop() {
		signal.ticker.cache = []kraken.TickerData{
			liquidityRow("BTC/USD", 999, 1001, 2, 2, 1, 1000),
			liquidityRow("ETH/USD", 99, 101, 5, 5, 100, 100),
			liquidityRow("SOL/USD", 99, 101, 5, 5, 100, 100),
		}
		_ = signal.Measure(types.NewThesis(nil))
	}
}
