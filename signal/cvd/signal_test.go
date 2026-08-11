package cvd

import (
	"context"
	"math"
	"sync"
	"testing"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	. "github.com/smartystreets/goconvey/convey"
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
		appendMarketTickers(causalCut, causalTicker)
		So(signal.Measure(causalCut), ShouldBeEmpty)

		futureCut := types.NewThesis(t.Context(), nil)
		appendMarketTickers(futureCut, futureTicker)
		appendMarketTrades(futureCut, trade)
		measurements := signal.Measure(futureCut)

		Convey("It uses the older midpoint, emits the preserved contract, and commits once", func() {
			So(measurements, ShouldHaveLength, 1)
			So(signal.seenTrade(trade), ShouldBeTrue)
			raw, exists := signal.midpoints.Load("BTC/USD")
			So(exists, ShouldBeTrue)
			So(raw.([]midpointObservation), ShouldHaveLength, 2)
			measurement := measurements[0]
			So(measurement.At, ShouldResemble, trade.Timestamp)
			So(measurement.Metrics, ShouldHaveLength, 11)
			_, hasSNR := measurement.Metrics[types.MetricKey(
				types.MetricSNR, types.SideNone,
			)]
			So(hasSNR, ShouldBeTrue)
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
			So(*net.Normalized, ShouldEqual,
				measurement.Sample(types.MetricNetFraction, types.SideNone).Raw)

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
		appendMarketTickers(futureCut,
			cvdTicker(99, 101, base.Add(time.Second)),
		)
		appendMarketTrades(futureCut, trade)

		Convey("It defers without consuming the trade, then resolves after causal evidence arrives", func() {
			So(signal.Measure(futureCut), ShouldBeEmpty)
			So(signal.seenTrade(trade), ShouldBeFalse)

			causalCut := types.NewThesis(t.Context(), nil)
			appendMarketTickers(causalCut,
				cvdTicker(98, 100, base),
			)
			appendMarketTrades(causalCut, trade)
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
		appendMarketTickers(cut, bitcoinTicker, altTicker)
		cut.AppendTrade(bitcoinSecond)
		cut.AppendTrade(bitcoinFirst)
		cut.AppendTrade(altSecond)
		cut.AppendTrade(altFirst)

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
			So(signal.Measure(cut), ShouldBeEmpty)
		})
	})
}

func appendMarketTickers(thesis *types.Thesis, rows ...kraken.TickerData) {
	for _, row := range rows {
		thesis.AppendTicker(row)
	}
}

func appendMarketTrades(thesis *types.Thesis, rows ...kraken.TradeData) {
	for _, row := range rows {
		thesis.AppendTrade(row)
	}
}

func TestCVDMeasurements(t *testing.T) {
	Convey("Given multi-observation aggressor and price-response evidence", t, func() {
		measurement := (&Signal{}).cvdMeasurements(
			cvdTrade(1, "sell", 100, time.Unix(1_700_003_400, 0).UTC()),
			100.5,
			equation.FlowOutput{
				Absorption: 0.2, Drive: 0.3, Balance: 0.5, Starvation: 0,
				Value: 0.5, Net: -25, NetFraction: 0.25,
			},
			4,
		)[0]

		Convey("It should normalize every defined flow family and signed net", func() {
			for _, metric := range []types.MetricType{
				types.MetricAbsorption,
				types.MetricDrive,
				types.MetricBalance,
				types.MetricStarvation,
				types.MetricStrength,
				types.MetricNetFraction,
				types.MetricNet,
			} {
				So(measurement.Sample(metric, types.SideNone).Normalized, ShouldNotBeNil)
			}

			So(*measurement.Sample(types.MetricNet, types.SideNone).Normalized,
				ShouldEqual, -0.25)
			So(measurement.Sample(types.MetricSNR, types.SideNone).Raw,
				ShouldEqual, (0.5-math.Sqrt(0.2*0.2+0.3*0.3))/0.5)
			So(measurement.Sample(types.MetricMidpoint, types.SideNone), ShouldResemble,
				types.MetricSample{Raw: 100.5, Unit: types.UnitQuoteCurrency})
			So(measurement.Sample(types.MetricTradePrice, types.SideNone), ShouldResemble,
				types.MetricSample{Raw: 100, Unit: types.UnitQuoteCurrency})
			So(measurement.Sample(types.MetricTradeQuantity, types.SideNone), ShouldResemble,
				types.MetricSample{Raw: 2, Unit: types.UnitBaseCurrency})
		})
	})

	Convey("Given equally strong competing CVD regimes", t, func() {
		measurement := (&Signal{}).cvdMeasurements(
			cvdTrade(2, "buy", 100, time.Unix(1_700_003_401, 0).UTC()),
			100,
			equation.FlowOutput{Absorption: 0.5, Drive: 0.5, Value: 0.5},
			4,
		)[0]

		Convey("It should report no separation between the tied hypotheses", func() {
			So(measurement.Sample(types.MetricSNR, types.SideNone).Raw,
				ShouldEqual, 0.0)
		})
	})

	Convey("Given a flow score one representable value outside its probability domain", t, func() {
		Convey("It should fail loudly before corrupt evidence reaches SNR", func() {
			So(func() {
				(&Signal{}).cvdMeasurements(
					cvdTrade(3, "buy", 100, time.Unix(1_700_003_402, 0).UTC()),
					100,
					equation.FlowOutput{
						Drive: math.Nextafter(1, 2),
					},
					2,
				)
			}, ShouldPanic)
		})
	})
}

func TestNormalizedFlowMetric(t *testing.T) {
	Convey("Given a bounded flow family", t, func() {
		Convey("It should expose balance immediately but gate price-response families", func() {
			So(*normalizedFlowMetric(types.MetricBalance, 0.25, 1),
				ShouldEqual, 0.25)
			So(normalizedFlowMetric(types.MetricAbsorption, 0.25, 1),
				ShouldBeNil)
			So(*normalizedFlowMetric(types.MetricAbsorption, 0.25, 2),
				ShouldEqual, 0.25)
		})

		Convey("It should retain exact boundaries and reject their adjacent exterior values", func() {
			So(*normalizedFlowMetric(types.MetricDrive, 0, 2), ShouldEqual, 0.0)
			So(*normalizedFlowMetric(types.MetricDrive, 1, 2), ShouldEqual, 1.0)
			So(normalizedFlowMetric(
				types.MetricDrive,
				math.Nextafter(0, -1),
				2,
			), ShouldBeNil)
			So(normalizedFlowMetric(
				types.MetricDrive,
				math.Nextafter(1, 2),
				2,
			), ShouldBeNil)
		})
	})
}

func TestNormalizedSignedNet(t *testing.T) {
	Convey("Given signed quote flow and its gross-notional fraction", t, func() {
		Convey("It should preserve direction, magnitude, and exact zero", func() {
			So(*normalizedSignedNet(25, 0.25, 1), ShouldEqual, 0.25)
			So(*normalizedSignedNet(-25, 0.25, 1), ShouldEqual, -0.25)
			So(*normalizedSignedNet(0, 0.25, 1), ShouldEqual, 0.0)
		})

		Convey("It should reject a fraction outside executed gross notional", func() {
			So(normalizedSignedNet(25, math.Nextafter(0, -1), 1), ShouldBeNil)
			So(normalizedSignedNet(25, math.Nextafter(1, 2), 1), ShouldBeNil)
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
		_ = signal.cvdMeasurements(row, 100.5, output, 4)
	}
}
