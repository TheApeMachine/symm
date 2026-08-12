package liquidity

import (
	"context"
	"math"
	"sync"
	"testing"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/types"
)

func ticker(
	symbol string,
	price, quantity, volume float64,
	at time.Time,
) kraken.TickerData {
	return kraken.TickerData{
		Symbol:    symbol,
		Bid:       decimal.NewFromFloat64(price - 0.5),
		BidQty:    quantity,
		Ask:       decimal.NewFromFloat64(price + 0.5),
		AskQty:    quantity,
		Last:      decimal.NewFromFloat64(price),
		Volume:    volume,
		Vwap:      price,
		Timestamp: at,
	}
}

func measurementFor(
	measurements []*types.Measurement,
	symbol string,
) *types.Measurement {
	for _, measurement := range measurements {
		if measurement.Symbol == symbol {
			return measurement
		}
	}

	return nil
}

func TestMeasure(t *testing.T) {
	Convey("Given a multi-leg executable-depth cohort", t, func() {
		signal := &Signal{ctx: context.Background(), observations: &sync.Map{}}
		market := types.NewSymbol("THIN/USD", nil)
		start := time.Unix(1_700_000_000, 0).UTC()
		firstLeg := []kraken.TickerData{
			ticker("THIN/USD", 100, 1, 10, start),
			ticker("PEER-A/USD", 100, 10, 20, start),
			ticker("PEER-B/USD", 100, 12, 30, start),
			ticker("PEER-C/USD", 100, 14, 40, start),
		}
		secondLeg := []kraken.TickerData{
			ticker("THIN/USD", 101, 1, 11, start.Add(time.Second)),
			ticker("PEER-A/USD", 101, 10, 21, start.Add(time.Second)),
			ticker("PEER-B/USD", 101, 12, 31, start.Add(time.Second)),
			ticker("PEER-C/USD", 101, 14, 41, start.Add(time.Second)),
		}

		appendTickers(market, firstLeg...)
		firstMeasurements := signal.Measure(market)
		So(firstMeasurements, ShouldHaveLength, 1)
		appendTickers(market, secondLeg...)
		measurements := signal.Measure(market)

		Convey("It should normalize from the first complete cohort", func() {
			first := measurementFor(firstMeasurements, "THIN/USD")
			So(first.Sample(types.MetricExecutableTouchDepth, types.SideNone).Normalized,
				ShouldNotBeNil)
		})

		Convey("It should use a leave-one-out robust baseline", func() {
			thin := measurementFor(measurements, "THIN/USD")
			So(thin, ShouldNotBeNil)
			depth := thin.Metrics[types.MetricKey(types.MetricExecutableTouchDepth, types.SideNone)]
			median := thin.Metrics[types.MetricKey(types.MetricExecutableTouchDepthMedian, types.SideNone)].Raw
			relative := thin.Metrics[types.MetricKey(types.MetricRelativeTouchDepth, types.SideNone)].Raw
			scarcity := thin.Metrics[types.MetricKey(types.MetricScarcityScore, types.SideNone)].Raw
			So(depth.Raw, ShouldEqual, 101.0)
			So(median, ShouldEqual, 1212.0)
			So(relative, ShouldEqual, 101.0/1212.0)
			So(depth.Normalized, ShouldNotBeNil)
			So(*depth.Normalized, ShouldEqual, 101.0/(101.0+1212.0))
			So(*thin.Sample(types.MetricRelativeTouchDepth, types.SideNone).Normalized,
				ShouldEqual, (101.0/1212.0)/(1.0+101.0/1212.0))
			So(scarcity, ShouldEqual, 1111.0/(1111.0+202.0))
			reported := thin.Sample(
				types.MetricReportedVolumeNotional,
				types.SideNone,
			)
			depthEvidence := *depth.Normalized
			relativeEvidence := *thin.Sample(
				types.MetricRelativeTouchDepth,
				types.SideNone,
			).Normalized
			reportedEvidence := *reported.Normalized
			available := math.Sqrt(
				(depthEvidence*depthEvidence +
					relativeEvidence*relativeEvidence +
					reportedEvidence*reportedEvidence) / 3,
			)
			expectedSNR := (scarcity - available) / scarcity
			So(thin.Sample(types.MetricSNR, types.SideNone).Raw,
				ShouldEqual, expectedSNR)
			So(thin.Sample(types.MetricReportedVolumeNotional, types.SideNone).Normalized,
				ShouldNotBeNil)
			So(thin.Sample(types.MetricExecutableTouchDepthMedian, types.SideNone).Normalized,
				ShouldNotBeNil)
		})

		Convey("It should not emit unchanged cached observations", func() {
			So(signal.Measure(market), ShouldBeEmpty)
		})
	})

	Convey("Given valid executable depth but missing reported turnover", t, func() {
		signal := &Signal{ctx: context.Background(), observations: &sync.Map{}}
		market := types.NewSymbol("NO-VOLUME/USD", nil)
		start := time.Unix(1_700_000_050, 0).UTC()

		for leg := range 2 {
			at := start.Add(time.Duration(leg) * time.Second)
			appendTickers(market,
				ticker("NO-VOLUME/USD", 100, 2, 0, at),
				ticker("PEER-A/USD", 100, 4, 20, at),
				ticker("PEER-B/USD", 100, 6, 30, at),
				ticker("PEER-C/USD", 100, 8, 40, at),
			)
			_ = signal.Measure(market)
		}

		measurements := signal.Measure(market)
		So(measurements, ShouldBeEmpty)
		at := start.Add(2 * time.Second)
		appendTickers(market,
			ticker("NO-VOLUME/USD", 100, 2, 0, at),
			ticker("PEER-A/USD", 100, 4, 20, at),
			ticker("PEER-B/USD", 100, 6, 30, at),
			ticker("PEER-C/USD", 100, 8, 40, at),
		)
		measurements = signal.Measure(market)

		Convey("It should keep depth usable while turnover normalization stays absent", func() {
			measurement := measurementFor(measurements, "NO-VOLUME/USD")
			So(measurement.Sample(
				types.MetricExecutableTouchDepth,
				types.SideNone,
			).Normalized, ShouldNotBeNil)
			So(measurement.Sample(
				types.MetricReportedVolumeNotional,
				types.SideNone,
			).Normalized, ShouldBeNil)
		})
	})
}

func TestNormalizedRelativeLiquidity(t *testing.T) {
	Convey("Given a leave-one-out depth ratio against empirical parity", t, func() {
		Convey("It should map zero, parity, and an exact multiple to their shares", func() {
			So(*normalizedRelativeLiquidity(0), ShouldEqual, 0.0)
			So(*normalizedRelativeLiquidity(1), ShouldEqual, 0.5)
			So(*normalizedRelativeLiquidity(3), ShouldEqual, 3.0/4.0)
		})

		Convey("It should reject the nearest negative representable ratio", func() {
			So(normalizedRelativeLiquidity(math.Nextafter(0, -1)), ShouldBeNil)
		})
	})
}

func TestLiquidityCohortMedian(t *testing.T) {
	Convey("Given positive, zero, and extreme cohort observations", t, func() {
		peers := []liquidityPeer{
			{symbol: "ZERO/USD"},
			{symbol: "A/USD", observation: liquidityObservation{
				executableDepth: 2, quoteNotional: 20,
			}},
			{symbol: "B/USD", observation: liquidityObservation{
				executableDepth: 4, quoteNotional: 40,
			}},
			{symbol: "OUTLIER/USD", observation: liquidityObservation{
				executableDepth: 100, quoteNotional: 1000,
			}},
		}

		Convey("It should exclude missing values and retain the robust middle observation", func() {
			depth, depthReady := liquidityCohortMedian(peers, true)
			notional, notionalReady := liquidityCohortMedian(peers, false)
			So(depth, ShouldEqual, 4.0)
			So(depthReady, ShouldBeTrue)
			So(notional, ShouldEqual, 40.0)
			So(notionalReady, ShouldBeTrue)
		})
	})
}

func TestNormalizedLiquidityRatio(t *testing.T) {
	Convey("Given current liquidity and a positive cohort baseline", t, func() {
		Convey("It should compute exact shares and preserve zero as measured", func() {
			So(*normalizedLiquidityRatio(0, 4), ShouldEqual, 0.0)
			So(*normalizedLiquidityRatio(4, 4), ShouldEqual, 0.5)
			So(*normalizedLiquidityRatio(12, 4), ShouldEqual, 12.0/16.0)
		})

		Convey("It should reject negative numerators and non-positive scales", func() {
			So(normalizedLiquidityRatio(math.Nextafter(0, -1), 4), ShouldBeNil)
			So(normalizedLiquidityRatio(4, 0), ShouldBeNil)
			So(normalizedLiquidityRatio(4, math.Nextafter(0, -1)), ShouldBeNil)
		})
	})
}

func TestNormalizedLiquidityScore(t *testing.T) {
	Convey("Given scarcity's closed probability domain", t, func() {
		Convey("It should retain both boundaries and reject adjacent exterior values", func() {
			So(*normalizedLiquidityScore(0), ShouldEqual, 0.0)
			So(*normalizedLiquidityScore(1), ShouldEqual, 1.0)
			So(normalizedLiquidityScore(math.Nextafter(0, -1)), ShouldBeNil)
			So(normalizedLiquidityScore(math.Nextafter(1, 2)), ShouldBeNil)
		})
	})
}

func TestLeaveOneOutLiquidity(t *testing.T) {
	Convey("Given a target, valid peers, and peers missing one evidence family", t, func() {
		peers := []liquidityPeer{
			{symbol: "TARGET/USD", observation: liquidityObservation{
				executableDepth: 1, quoteNotional: 10,
			}},
			{symbol: "A/USD", observation: liquidityObservation{
				executableDepth: 2, quoteNotional: 20,
			}},
			{symbol: "B/USD", observation: liquidityObservation{
				executableDepth: 4,
			}},
			{symbol: "C/USD", observation: liquidityObservation{
				quoteNotional: 60,
			}},
		}
		depths, notionals := leaveOneOutLiquidity("TARGET/USD", peers)

		Convey("It should exclude the target and never substitute missing peer evidence", func() {
			So(depths, ShouldResemble, []float64{2, 4})
			So(notionals, ShouldResemble, []float64{20, 60})
		})
	})
}

func appendTickers(market *types.Symbol, rows ...kraken.TickerData) {
	for _, row := range rows {
		market.AppendTicker(row)
	}
}

func BenchmarkMeasure(b *testing.B) {
	signal := &Signal{ctx: context.Background(), observations: &sync.Map{}}
	market := types.NewSymbol("AAA/USD", nil)
	start := time.Unix(1_700_000_100, 0).UTC()

	b.ReportAllocs()

	for index := range b.N {
		at := start.Add(time.Duration(index) * time.Second)
		appendTickers(market,
			ticker("AAA/USD", 100, 2, 10, at),
			ticker("BBB/USD", 100, 4, 20, at),
			ticker("CCC/USD", 100, 6, 30, at),
		)
		_ = signal.Measure(market)
	}
}
