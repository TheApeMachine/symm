package pumpdump

import (
	"context"
	"math"
	"sync"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/spf13/viper"
	spotbook "github.com/theapemachine/api-go/v2/pkg/book"
	"github.com/theapemachine/api-go/v2/pkg/decimal"
	"github.com/theapemachine/nomagique/equation"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/types"
)

func TestSeenTrade(t *testing.T) {
	Convey("Given an exact-once cursor for one pumpdump symbol", t, func() {
		signal := &Signal{lastTrade: &sync.Map{}}
		at := time.Unix(1_700_002_000, 0).UTC()
		first := kraken.TradeData{Symbol: "ALT/USD", TradeID: 51, Timestamp: at}
		second := kraken.TradeData{Symbol: "ALT/USD", TradeID: 52, Timestamp: at}
		regressed := kraken.TradeData{Symbol: "ALT/USD", TradeID: 53, Timestamp: at.Add(-time.Nanosecond)}

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

	Convey("Given same-time pumpdump trades without exchange IDs", t, func() {
		signal := &Signal{lastTrade: &sync.Map{}}
		trade := kraken.TradeData{Symbol: "ALT/USD", Timestamp: time.Unix(1_700_002_100, 0).UTC()}
		signal.commitTrade(trade)

		Convey("It documents intrinsic zero-ID indistinguishability", func() {
			So(signal.seenTrade(trade), ShouldBeTrue)
		})
	})
}

func TestMeasure(t *testing.T) {
	Convey("Given a multi-leg directional replay with causal quote evidence", t, func() {
		viper.Set("signals.pumpdump.baselineCapacity", 128)
		books := &pumpdumpBookSource{books: make(map[string]*spotbook.Book)}
		signal := &Signal{
			ctx:       context.Background(),
			algo:      equation.NewIgnition(128),
			books:     books,
			lastTrade: &sync.Map{},
		}
		thesis := types.NewThesis(t.Context(), nil)
		base := time.Unix(1_700_002_200, 0).UTC()
		var measurements []*types.Measurement
		expectedAlgorithm := equation.NewIgnition(128)
		expectedOutput := equation.IgnitionOutput{}
		expectedReady := false
		expectedMaturity := 0.0

		for index, price := range []float64{100, 101, 100, 102, 101, 104} {
			at := base.Add(time.Duration(index) * time.Second)
			books.books["BTC/USD"] = pumpdumpBook(
				"BTC/USD",
				price-0.5,
				price+0.5,
				at,
			)
			thesis.AppendTrade(pumpdumpTrade(int64(index+1), price, at))
			var err error
			expectedOutput, expectedReady, expectedMaturity, err = expectedAlgorithm.Measure(
				equation.IgnitionInput{
					At: at, Symbol: "BTC/USD", Last: price, Volume: 20,
					Ask: price + 0.5, Bid: price - 0.5,
				},
			)
			So(err, ShouldBeNil)
			measurements = signal.Measure(thesis)
		}

		Convey("It preserves legacy keys and publishes both dimensionless directional families", func() {
			So(measurements, ShouldHaveLength, 1)
			measurement := measurements[0]
			So(measurement.Metrics, ShouldHaveLength, 26)
			So(measurement.Maturity, ShouldEqual, expectedMaturity)
			So(measurement.Sample(types.MetricRVOL, types.SideNone).Unit,
				ShouldEqual, types.UnitDimensionless)
			So(measurement.Sample(types.MetricSpread, types.SideNone).Unit,
				ShouldEqual, types.UnitQuoteCurrency)
			So(*measurement.Sample(types.MetricSpread, types.SideNone).Normalized,
				ShouldEqual, 1.0/104.0)
			So(measurement.Sample(types.MetricPrecursor, types.SideBuy).Raw,
				ShouldEqual, expectedOutput.Buy.Precursor)
			So(measurement.Sample(types.MetricPrecursor, types.SideSell).Raw,
				ShouldEqual, expectedOutput.Sell.Precursor)
			So(measurement.Sample(types.MetricSNR, types.SideNone).Normalized,
				ShouldNotBeNil)

			So(measurement.Sample(types.MetricRVOL, types.SideNone).Raw,
				ShouldEqual, expectedOutput.RVOL)
			So(measurement.Sample(types.MetricSpread, types.SideNone).Raw,
				ShouldEqual, expectedOutput.Spread)

			expectedByMetric := map[types.MetricType]struct {
				unsided float64
				buy     float64
				sell    float64
			}{
				types.MetricPrecursor: {
					expectedOutput.Precursor,
					expectedOutput.Buy.Precursor,
					expectedOutput.Sell.Precursor,
				},
				types.MetricCompression: {
					expectedOutput.Compression,
					expectedOutput.Buy.Compression,
					expectedOutput.Sell.Compression,
				},
				types.MetricIgnition: {
					expectedOutput.Ignition,
					expectedOutput.Buy.Ignition,
					expectedOutput.Sell.Ignition,
				},
				types.MetricTrend: {
					expectedOutput.Trend,
					expectedOutput.Buy.Trend,
					expectedOutput.Sell.Trend,
				},
				types.MetricExhaustion: {
					expectedOutput.Exhaustion,
					expectedOutput.Buy.Exhaustion,
					expectedOutput.Sell.Exhaustion,
				},
				types.MetricStrength: {
					expectedOutput.Strength,
					expectedOutput.Buy.Strength,
					expectedOutput.Sell.Strength,
				},
			}

			for _, metric := range []types.MetricType{
				types.MetricPrecursor,
				types.MetricCompression,
				types.MetricIgnition,
				types.MetricTrend,
				types.MetricExhaustion,
				types.MetricStrength,
			} {
				So(measurement.Sample(metric, types.SideBuy).Unit,
					ShouldEqual, types.UnitDimensionless)
				So(measurement.Sample(metric, types.SideSell).Unit,
					ShouldEqual, types.UnitDimensionless)
				So(measurement.Sample(metric, types.SideNone).Unit,
					ShouldEqual, types.UnitDimensionless)
				So(measurement.Sample(metric, types.SideNone).Normalized,
					ShouldNotBeNil)
				So(measurement.Sample(metric, types.SideNone).Raw,
					ShouldEqual, expectedByMetric[metric].unsided)
				So(measurement.Sample(metric, types.SideBuy).Raw,
					ShouldEqual, expectedByMetric[metric].buy)
				So(measurement.Sample(metric, types.SideSell).Raw,
					ShouldEqual, expectedByMetric[metric].sell)
				So(measurement.Sample(metric, types.SideNone).Normalized,
					ShouldResemble, normalizedIgnitionEvidence(
						metric,
						expectedByMetric[metric].unsided,
						expectedReady,
					))
			}
		})
	})
}

func TestNormalizedIgnitionEvidence(t *testing.T) {
	Convey("Given an ignition score before its empirical baseline is ready", t, func() {
		Convey("It should leave the normalized value absent", func() {
			So(normalizedIgnitionEvidence(
				types.MetricIgnition, 0, false,
			), ShouldBeNil)
		})
	})

	Convey("Given a ready empirical score that is genuinely zero", t, func() {
		Convey("It should retain zero as measured evidence", func() {
			value := normalizedIgnitionEvidence(types.MetricIgnition, 0, true)
			So(value, ShouldNotBeNil)
			So(*value, ShouldEqual, 0.0)
		})
	})

	Convey("Given evidence equal to its empirical ratio baseline", t, func() {
		Convey("It should map parity to the center of the unit interval", func() {
			value := normalizedIgnitionEvidence(types.MetricPrecursor, 1, true)
			So(value, ShouldNotBeNil)
			So(*value, ShouldEqual, 0.5)

			trend := normalizedIgnitionEvidence(types.MetricTrend, 1, true)
			So(trend, ShouldNotBeNil)
			So(*trend, ShouldEqual, 0.5)
		})
	})

	Convey("Given exact boundaries for bounded and unbounded ignition families", t, func() {
		Convey("It should retain bounded scores and squash unbounded evidence", func() {
			So(*normalizedIgnitionEvidence(types.MetricCompression, 0, true),
				ShouldEqual, 0.0)
			So(*normalizedIgnitionEvidence(types.MetricCompression, 1, true),
				ShouldEqual, 1.0)
			So(*normalizedIgnitionEvidence(types.MetricRVOL, 3, true),
				ShouldEqual, 3.0/4.0)
		})

		Convey("It should reject negative evidence and bounded overflow", func() {
			So(normalizedIgnitionEvidence(
				types.MetricRVOL,
				math.Nextafter(0, -1),
				true,
			), ShouldBeNil)
			So(normalizedIgnitionEvidence(
				types.MetricCompression,
				math.Nextafter(1, 2),
				true,
			), ShouldBeNil)
			So(normalizedIgnitionEvidence(
				types.MetricExhaustion,
				math.Nextafter(1, 2),
				true,
			), ShouldBeNil)
		})
	})
}

func TestNormalizedSpread(t *testing.T) {
	Convey("Given executable spread and its causal midpoint", t, func() {
		Convey("It should return the exact relative spread", func() {
			So(*normalizedSpread(1, 100), ShouldEqual, 1.0/100.0)
			So(*normalizedSpread(2, 100), ShouldEqual, 2.0/100.0)
		})

		Convey("It should reject absent spread and non-positive midpoint", func() {
			So(normalizedSpread(0, 100), ShouldBeNil)
			So(normalizedSpread(math.Nextafter(0, -1), 100), ShouldBeNil)
			So(normalizedSpread(1, 0), ShouldBeNil)
			So(normalizedSpread(1, math.Nextafter(0, -1)), ShouldBeNil)
		})
	})
}

type pumpdumpBookSource struct {
	books map[string]*spotbook.Book
}

func (source *pumpdumpBookSource) Book(symbol string, read func(*spotbook.Book)) {
	read(source.books[symbol])
}

func pumpdumpTrade(id int64, price float64, at time.Time) kraken.TradeData {
	return kraken.TradeData{
		Symbol: "BTC/USD", Side: "buy", Price: *decimal.NewFromFloat64(price),
		Qty: 20, TradeID: id, Timestamp: at,
	}
}

func pumpdumpBook(symbol string, bid, ask float64, at time.Time) *spotbook.Book {
	managed := spotbook.New()
	managed.Name = symbol
	managed.NoBookCrossing = false
	managed.Update(&spotbook.UpdateOptions{
		Direction: spotbook.Bid, Price: decimal.NewFromFloat64(bid),
		Quantity: decimal.NewFromInt64(10), Timestamp: at,
	})
	managed.Update(&spotbook.UpdateOptions{
		Direction: spotbook.Ask, Price: decimal.NewFromFloat64(ask),
		Quantity: decimal.NewFromInt64(10), Timestamp: at,
	})

	return managed
}

func BenchmarkNormalizedIgnitionEvidence(b *testing.B) {
	b.ReportAllocs()

	for b.Loop() {
		_ = normalizedIgnitionEvidence(types.MetricIgnition, 1.25, true)
	}
}
