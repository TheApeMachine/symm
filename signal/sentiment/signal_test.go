package sentiment

import (
	"context"
	"math"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/types"
)

func ticker(symbol string, price float64, at time.Time) kraken.TickerData {
	return kraken.TickerData{
		Symbol:    symbol,
		Last:      decimal.NewFromFloat64(price),
		Timestamp: at,
	}
}

func measurementFor(
	measurements []types.Measurement,
	symbol string,
) *types.Measurement {
	for index := range measurements {
		if measurements[index].Symbol == symbol {
			return &measurements[index]
		}
	}

	return nil
}

func TestMeasure(t *testing.T) {
	Convey("Given a causal multi-leg return cohort", t, func() {
		signal := &Signal{ctx: context.Background(), observations: &sync.Map{}}
		aaa := types.NewSymbol("AAA/USD", nil)
		bbb := types.NewSymbol("BBB/USD", nil)
		ccc := types.NewSymbol("CCC/USD", nil)
		cohort := []*types.Symbol{aaa, bbb, ccc}
		start := time.Unix(1_700_000_000, 0).UTC()

		firstLeg := []kraken.TickerData{
			ticker("AAA/USD", 100, start),
			ticker("BBB/USD", 100, start),
			ticker("CCC/USD", 100, start),
		}
		secondLeg := []kraken.TickerData{
			ticker("AAA/USD", 110, start.Add(time.Second)),
			ticker("BBB/USD", 101, start.Add(time.Second)),
			ticker("CCC/USD", 99, start.Add(time.Second)),
		}
		thirdLeg := []kraken.TickerData{
			ticker("AAA/USD", 121, start.Add(2*time.Second)),
			ticker("BBB/USD", 100, start.Add(2*time.Second)),
			ticker("CCC/USD", 98, start.Add(2*time.Second)),
		}

		appendTickersBySymbol(cohort, firstLeg)
		err := signal.MeasureCohort(cohort)
		So(err, ShouldBeNil)
		So(slices.Collect(aaa.MarketMeasurements("category")), ShouldHaveLength, 1)
		appendTickersBySymbol(cohort, secondLeg)
		err = signal.MeasureCohort(cohort)
		So(err, ShouldBeNil)
		So(slices.Collect(aaa.MarketMeasurements("category")), ShouldHaveLength, 1)
		appendTickersBySymbol(cohort, thirdLeg)
		err = signal.MeasureCohort(cohort)
		So(err, ShouldBeNil)

		Convey("It should use consecutive log returns and signed breadth", func() {
			measurements := slices.Collect(aaa.MarketMeasurements("category"))
			leader := measurementFor(measurements, "AAA/USD")
			So(leader, ShouldNotBeNil)
			change := leader.Metrics[types.MetricKey(types.MetricChange, types.SideNone)].Raw
			breadth := leader.Metrics[types.MetricKey(types.MetricBreadth, types.SideNone)].Raw
			expectedLeader := math.Log(121.0 / 110.0)
			firstPeer := math.Abs(math.Log(100.0 / 101.0))
			secondPeer := math.Abs(math.Log(98.0 / 99.0))
			peerMedian := (firstPeer + secondPeer) / 2
			peerDispersion := math.Abs(firstPeer-secondPeer) / 2
			excess := expectedLeader - peerMedian
			expectedLeaderEvidence := excess / (excess + peerDispersion)
			So(change, ShouldEqual, expectedLeader)
			So(breadth, ShouldEqual, -1.0/3.0)
			So(leader.Metrics[types.MetricKey(
				types.MetricLeaderEvidence,
				types.SideNone,
			)].Raw, ShouldEqual, expectedLeaderEvidence)
			So(leader.Metrics[types.MetricKey(
				types.MetricDivergentScore,
				types.SideNone,
			)].Raw, ShouldEqual, expectedLeaderEvidence)
			So(leader.Sample(types.MetricChange, types.SideNone).Normalized,
				ShouldNotBeNil)
			So(*leader.Sample(types.MetricBreadth, types.SideNone).Normalized,
				ShouldEqual, -1.0/3.0)
			So(leader.Sample(types.MetricHypothesisSeparation, types.SideNone).Raw,
				ShouldEqual, 1.0)
			So(leader.Metrics, ShouldHaveLength, 11)
		})

		Convey("It should reject repeated latest-value cache entries", func() {
			err := signal.MeasureCohort(cohort)
			So(err, ShouldBeNil)
			So(slices.Collect(aaa.MarketMeasurements("category")), ShouldBeEmpty)
		})
	})

	Convey("Given a cohort with real cadence but no return dispersion", t, func() {
		signal := &Signal{ctx: context.Background(), observations: &sync.Map{}}
		aaa := types.NewSymbol("AAA/USD", nil)
		bbb := types.NewSymbol("BBB/USD", nil)
		cohort := []*types.Symbol{aaa, bbb}
		start := time.Unix(1_700_000_100, 0).UTC()
		appendTickersBySymbol(cohort, []kraken.TickerData{
			ticker("AAA/USD", 100, start),
			ticker("BBB/USD", 100, start),
		})
		err := signal.MeasureCohort(cohort)
		So(err, ShouldBeNil)
		So(slices.Collect(aaa.MarketMeasurements("category")), ShouldHaveLength, 1)
		appendTickersBySymbol(cohort, []kraken.TickerData{
			ticker("AAA/USD", 100, start.Add(time.Second)),
			ticker("BBB/USD", 100, start.Add(time.Second)),
		})
		err = signal.MeasureCohort(cohort)
		So(err, ShouldBeNil)
		measurements := slices.Collect(aaa.MarketMeasurements("category"))

		Convey("It should leave the scale-dependent return unnormalized", func() {
			So(len(measurements), ShouldEqual, 1)
			So(measurements[0].Sample(types.MetricChange, types.SideNone).Normalized,
				ShouldBeNil)
			So(measurements[0].Sample(types.MetricHypothesisSeparation, types.SideNone).Raw,
				ShouldEqual, 0.0)
		})
	})

	Convey("Given ticker history retained across market epochs", t, func() {
		signal := &Signal{ctx: context.Background(), observations: &sync.Map{}}
		market := types.NewSymbol("AAA/USD", nil)
		start := time.Unix(1_700_000_200, 0).UTC()
		market.AppendTicker(ticker("AAA/USD", 100, start))

		err := signal.Measure(market)
		So(err, ShouldBeNil)
		So(slices.Collect(market.MarketMeasurements("category")), ShouldHaveLength, 1)

		market.AppendTicker(ticker("AAA/USD", 101, start.Add(time.Second)))
		err = signal.Measure(market)
		So(err, ShouldBeNil)
		measurements := slices.Collect(market.MarketMeasurements("category"))

		Convey("It should skip the observed row and measure the newer return", func() {
			So(measurements, ShouldHaveLength, 1)
			change := measurements[0].Sample(types.MetricChange, types.SideNone)
			So(change.Raw, ShouldEqual, math.Log(101.0/100.0))
			So(change.Normalized, ShouldNotBeNil)
		})
	})
}

func TestSentimentStatistics(t *testing.T) {
	Convey("Given exact cross-sectional returns with one leader and one dissenter", t, func() {
		peers := []sentimentPeer{
			{symbol: "A/USD", observation: returnObservation{change: 0.5}},
			{symbol: "B/USD", observation: returnObservation{change: 0.25}},
			{symbol: "C/USD", observation: returnObservation{change: -0.125}},
		}
		summary := sentimentStatistics(peers)

		Convey("It should produce every robust cohort statistic exactly", func() {
			So(summary.leader, ShouldEqual, "A/USD")
			So(summary.leaderMagnitude, ShouldEqual, 0.5)
			So(summary.magnitudeBaseline, ShouldEqual, 0.25)
			So(summary.scaleReady, ShouldBeTrue)
			So(summary.breadth, ShouldEqual, 1.0/3.0)
			So(summary.surge, ShouldEqual, 2.0/3.0)
			So(summary.slump, ShouldEqual, 0.0)
			So(summary.relativeLead, ShouldEqual, 4.0/7.0)
			So(summary.leaderEvidence, ShouldEqual, 5.0/6.0)
			So(summary.divergence, ShouldEqual, 5.0/12.0)
		})
	})

	Convey("Given equal-magnitude leaders", t, func() {
		peers := []sentimentPeer{
			{symbol: "A/USD", observation: returnObservation{change: 0.25}},
			{symbol: "B/USD", observation: returnObservation{change: -0.25}},
		}
		summary := sentimentStatistics(peers)

		Convey("It should use stable cohort order and claim no unsupported excess", func() {
			So(summary.leader, ShouldEqual, "A/USD")
			So(summary.leaderEvidence, ShouldEqual, 0.0)
			So(summary.divergence, ShouldEqual, 0.0)
			So(summary.breadth, ShouldEqual, 0.0)
			So(summary.surge, ShouldEqual, 0.0)
			So(summary.slump, ShouldEqual, 0.0)
		})
	})
}

func TestNormalizedSentimentMetric(t *testing.T) {
	Convey("Given signed returns, leadership magnitude, and bounded cohort scores", t, func() {
		Convey("It should normalize scale-dependent values against the exact baseline", func() {
			So(*normalizedSentimentMetric(types.MetricChange, -0.5, 0.25),
				ShouldEqual, -0.5/(0.5+0.25))
			So(*normalizedSentimentMetric(types.MetricLeaderStrength, 0.5, 0.25),
				ShouldEqual, 0.5/(0.5+0.25))
			So(normalizedSentimentMetric(types.MetricChange, 0.5, 0), ShouldBeNil)
		})

		Convey("It should enforce signed and unsigned score boundaries exactly", func() {
			So(*normalizedSentimentMetric(types.MetricBreadth, -1, 0),
				ShouldEqual, -1.0)
			So(*normalizedSentimentMetric(types.MetricBreadth, 1, 0),
				ShouldEqual, 1.0)
			So(*normalizedSentimentMetric(types.MetricSurgeScore, 0, 0),
				ShouldEqual, 0.0)
			So(*normalizedSentimentMetric(types.MetricSurgeScore, 1, 0),
				ShouldEqual, 1.0)
			So(normalizedSentimentMetric(
				types.MetricBreadth,
				math.Nextafter(-1, -2),
				0,
			), ShouldBeNil)
			So(normalizedSentimentMetric(
				types.MetricBreadth,
				math.Nextafter(1, 2),
				0,
			), ShouldBeNil)
			So(normalizedSentimentMetric(
				types.MetricSurgeScore,
				math.Nextafter(0, -1),
				0,
			), ShouldBeNil)
			So(normalizedSentimentMetric(
				types.MetricSurgeScore,
				math.Nextafter(1, 2),
				0,
			), ShouldBeNil)
		})
	})
}

func appendTickers(market *types.Symbol, rows ...kraken.TickerData) {
	for _, row := range rows {
		market.AppendTicker(row)
	}
}

func appendTickersBySymbol(
	symbols []*types.Symbol,
	rows []kraken.TickerData,
) {
	bySymbol := make(map[string]*types.Symbol, len(symbols))

	for _, symbol := range symbols {
		bySymbol[symbol.Symbol] = symbol
	}

	for _, row := range rows {
		if owner, found := bySymbol[row.Symbol]; found {
			owner.AppendTicker(row)
		}
	}
}

func BenchmarkSentimentStatistics(b *testing.B) {
	peers := []sentimentPeer{
		{symbol: "AAA/USD", observation: returnObservation{change: 0.03}},
		{symbol: "BBB/USD", observation: returnObservation{change: 0.01}},
		{symbol: "CCC/USD", observation: returnObservation{change: -0.02}},
		{symbol: "DDD/USD", observation: returnObservation{change: 0.015}},
	}

	b.ReportAllocs()

	for b.Loop() {
		_ = sentimentStatistics(peers)
	}
}

func BenchmarkMeasure(b *testing.B) {
	market := types.NewSymbol("AAA/USD", nil)
	start := time.Unix(1_700_000_300, 0).UTC()

	for index := range 256 {
		market.AppendTicker(ticker(
			"AAA/USD",
			100+float64(index)/100,
			start.Add(time.Duration(index)*time.Second),
		))
	}

	b.ReportAllocs()

	for b.Loop() {
		signal := &Signal{ctx: context.Background(), observations: &sync.Map{}}
		_ = signal.Measure(market)
	}
}
