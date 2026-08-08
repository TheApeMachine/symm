package cvd

import (
	"context"
	"sync"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/api-go/v2/pkg/decimal"
	"github.com/theapemachine/nomagique/algorithm"
	"github.com/theapemachine/nomagique/equation"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/types"
)

func TestSeenTrade(t *testing.T) {
	Convey("Given an exact-once cursor for one CVD symbol", t, func() {
		signal := &Signal{lastTrade: &sync.Map{}}
		at := time.Unix(1_700_003_000, 0).UTC()
		first := kraken.TradeData{Symbol: "ALT/USD", TradeID: 61, Timestamp: at}
		second := kraken.TradeData{Symbol: "ALT/USD", TradeID: 62, Timestamp: at}
		regressed := kraken.TradeData{Symbol: "ALT/USD", TradeID: 63, Timestamp: at.Add(-time.Nanosecond)}

		Convey("It accepts distinct same-time IDs and rejects replay or regression", func() {
			So(signal.seenTrade(first), ShouldBeFalse)
			signal.commitTrade(first)
			So(signal.seenTrade(first), ShouldBeTrue)
			So(signal.seenTrade(second), ShouldBeFalse)
			signal.commitTrade(second)
			So(signal.seenTrade(second), ShouldBeTrue)
			So(signal.seenTrade(regressed), ShouldBeTrue)
		})
	})

	Convey("Given same-time CVD trades without exchange IDs", t, func() {
		signal := &Signal{lastTrade: &sync.Map{}}
		trade := kraken.TradeData{Symbol: "ALT/USD", Timestamp: time.Unix(1_700_003_100, 0).UTC()}
		signal.commitTrade(trade)

		Convey("It documents intrinsic zero-ID indistinguishability", func() {
			So(signal.seenTrade(trade), ShouldBeTrue)
		})
	})
}

func TestMeasure(t *testing.T) {
	Convey("Given a retained causal midpoint and a newer future ticker", t, func() {
		signal := &Signal{
			ctx:       context.Background(),
			sample:    algorithm.NewTradeFlowSample(),
			flow:      equation.NewFlow(),
			midpoints: &sync.Map{},
			lastTrade: &sync.Map{},
		}
		base := time.Unix(1_700_003_200, 0).UTC()
		causalTicker := cvdTicker(99, 101, base)
		futureTicker := cvdTicker(109, 111, base.Add(3*time.Second))
		trade := cvdTrade(71, "buy", 101, base.Add(2*time.Second))

		causalCut := types.NewThesis(t.Context(), nil)
		causalCut.Tickers.Store("BTC/USD", []kraken.TickerData{causalTicker})
		So(signal.Measure(causalCut), ShouldBeEmpty)

		futureCut := types.NewThesis(t.Context(), nil)
		futureCut.Tickers.Store("BTC/USD", []kraken.TickerData{futureTicker})
		futureCut.Trades.Store("BTC/USD", []kraken.TradeData{trade})
		measurements := signal.Measure(futureCut)

		Convey("It uses the older midpoint, emits the preserved contract, and commits once", func() {
			So(measurements, ShouldHaveLength, 1)
			So(signal.seenTrade(trade), ShouldBeTrue)
			raw, exists := signal.midpoints.Load("BTC/USD")
			So(exists, ShouldBeTrue)
			So(raw.([]midpointObservation), ShouldHaveLength, 2)
			measurement := measurements[0]
			So(measurement.At, ShouldResemble, trade.Timestamp)
			So(measurement.Metrics, ShouldHaveLength, 7)
			So(measurement.Sample(types.MetricNet, types.SideNone).Unit,
				ShouldEqual, types.UnitQuoteCurrency)

			for _, metric := range []types.MetricType{
				types.MetricBalance,
				types.MetricStrength,
				types.MetricNetFraction,
			} {
				So(measurement.Sample(metric, types.SideNone).Unit,
					ShouldEqual, types.UnitDimensionless)
				So(measurement.Sample(metric, types.SideNone).Normalized,
					ShouldNotBeNil)
			}

			for _, metric := range []types.MetricType{
				types.MetricAbsorption,
				types.MetricDrive,
				types.MetricStarvation,
			} {
				So(measurement.Sample(metric, types.SideNone).Normalized,
					ShouldBeNil)
			}

			net := measurement.Sample(types.MetricNet, types.SideNone)
			So(net.Normalized, ShouldNotBeNil)
			So(*net.Normalized, ShouldAlmostEqual,
				measurement.Sample(types.MetricNetFraction, types.SideNone).Raw, 1e-12)

			So(signal.Measure(futureCut), ShouldBeEmpty)
		})
	})

	Convey("Given a trade with only future midpoint evidence", t, func() {
		signal := &Signal{
			ctx:       context.Background(),
			sample:    algorithm.NewTradeFlowSample(),
			flow:      equation.NewFlow(),
			midpoints: &sync.Map{},
			lastTrade: &sync.Map{},
		}
		base := time.Unix(1_700_003_300, 0).UTC()
		trade := cvdTrade(72, "sell", 100, base)
		futureCut := types.NewThesis(t.Context(), nil)
		futureCut.Tickers.Store("BTC/USD", []kraken.TickerData{
			cvdTicker(99, 101, base.Add(time.Second)),
		})
		futureCut.Trades.Store("BTC/USD", []kraken.TradeData{trade})

		Convey("It defers without consuming the trade, then resolves after causal evidence arrives", func() {
			So(signal.Measure(futureCut), ShouldBeEmpty)
			So(signal.seenTrade(trade), ShouldBeFalse)

			causalCut := types.NewThesis(t.Context(), nil)
			causalCut.Tickers.Store("BTC/USD", []kraken.TickerData{
				cvdTicker(98, 100, base),
			})
			causalCut.Trades.Store("BTC/USD", []kraken.TradeData{trade})
			So(signal.Measure(causalCut), ShouldHaveLength, 1)
			So(signal.seenTrade(trade), ShouldBeTrue)
		})
	})

	Convey("Given reversed trade batches for independent CVD symbols", t, func() {
		signal := &Signal{
			ctx:       context.Background(),
			sample:    algorithm.NewTradeFlowSample(),
			flow:      equation.NewFlow(),
			midpoints: &sync.Map{},
			lastTrade: &sync.Map{},
		}
		base := time.Unix(1_700_003_350, 0).UTC()
		bitcoinTicker := cvdTicker(99, 101, base)
		altTicker := cvdTicker(49, 51, base)
		altTicker.Symbol = "ALT/USD"
		bitcoinFirst := cvdTrade(73, "buy", 100, base.Add(time.Second))
		bitcoinSecond := cvdTrade(74, "sell", 101, base.Add(2*time.Second))
		altFirst := cvdTrade(75, "sell", 50, base.Add(time.Second))
		altFirst.Symbol = "ALT/USD"
		altSecond := cvdTrade(76, "buy", 51, base.Add(2*time.Second))
		altSecond.Symbol = "ALT/USD"
		cut := types.NewThesis(t.Context(), nil)
		cut.Tickers.Store("BTC/USD", []kraken.TickerData{bitcoinTicker})
		cut.Tickers.Store("ALT/USD", []kraken.TickerData{altTicker})
		cut.Trades.Store("BTC/USD", []kraken.TradeData{bitcoinSecond, bitcoinFirst})
		cut.Trades.Store("ALT/USD", []kraken.TradeData{altSecond, altFirst})

		Convey("It preserves causal order independently for each symbol", func() {
			measurements := signal.Measure(cut)
			So(measurements, ShouldHaveLength, 4)

			measurementsBySymbol := make(map[string][]*types.Measurement)

			for _, measurement := range measurements {
				measurementsBySymbol[measurement.Symbol] = append(
					measurementsBySymbol[measurement.Symbol], measurement,
				)
			}

			So(measurementsBySymbol["BTC/USD"], ShouldHaveLength, 2)
			So(measurementsBySymbol["ALT/USD"], ShouldHaveLength, 2)
			So(measurementsBySymbol["BTC/USD"][0].At.Before(
				measurementsBySymbol["BTC/USD"][1].At,
			), ShouldBeTrue)
			So(measurementsBySymbol["ALT/USD"][0].At.Before(
				measurementsBySymbol["ALT/USD"][1].At,
			), ShouldBeTrue)
			So(signal.Measure(cut), ShouldBeEmpty)
		})
	})
}

func TestCVDMeasurements(t *testing.T) {
	Convey("Given multi-observation aggressor and price-response evidence", t, func() {
		measurement := (&Signal{}).cvdMeasurements(
			cvdTrade(1, "sell", 100, time.Unix(1_700_003_400, 0).UTC()),
			equation.FlowOutput{
				Absorption: 0.2, Drive: 0.3, Balance: 0.5, Starvation: 0,
				Value: 0.5, Net: -25, NetFraction: 0.25,
			},
			4,
		)[0]

		Convey("It should normalize every defined flow family and signed net", func() {
			for _, sample := range measurement.Metrics {
				So(sample.Normalized, ShouldNotBeNil)
			}

			So(*measurement.Sample(types.MetricNet, types.SideNone).Normalized,
				ShouldAlmostEqual, -0.25, 1e-12)
		})
	})
}

func cvdTicker(bid, ask float64, at time.Time) kraken.TickerData {
	return kraken.TickerData{
		Symbol: "BTC/USD", Bid: decimal.NewFromFloat64(bid),
		Ask: decimal.NewFromFloat64(ask), Timestamp: at,
	}
}

func cvdTrade(id int64, side string, price float64, at time.Time) kraken.TradeData {
	return kraken.TradeData{
		Symbol: "BTC/USD", Side: side, Price: *decimal.NewFromFloat64(price),
		Qty: 2, TradeID: id, Timestamp: at,
	}
}

func BenchmarkCVDMeasurements(b *testing.B) {
	signal := &Signal{}
	row := cvdTrade(1, "sell", 100, time.Unix(1_700_003_500, 0).UTC())
	output := equation.FlowOutput{
		Absorption:  0.2,
		Drive:       0.3,
		Balance:     0.5,
		Value:       0.5,
		Net:         -25,
		NetFraction: 0.25,
	}

	b.ReportAllocs()

	for b.Loop() {
		_ = signal.cvdMeasurements(row, output, 4)
	}
}
