package sentiment

import (
	"math"
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
	Convey("Given a causal multi-leg return cohort", t, func() {
		signal := &Signal{observations: make(map[string]returnObservation)}
		thesis := types.NewThesis(nil)
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

		thesis.Tickers = tickerMap(firstLeg)
		So(signal.Measure(thesis), ShouldBeEmpty)
		thesis.Tickers = tickerMap(secondLeg)
		So(signal.Measure(thesis), ShouldHaveLength, 3)
		thesis.Tickers = tickerMap(thirdLeg)
		measurements := signal.Measure(thesis)

		Convey("It should use consecutive log returns and signed breadth", func() {
			leader := measurementFor(measurements, "AAA/USD")
			So(leader, ShouldNotBeNil)
			change := leader.Metrics[types.MetricKey(types.MetricChange, types.SideNone)].Raw
			breadth := leader.Metrics[types.MetricKey(types.MetricBreadth, types.SideNone)].Raw
			So(change, ShouldAlmostEqual, math.Log(121.0/110.0), 1e-12)
			So(breadth, ShouldAlmostEqual, -1.0/3.0, 1e-12)
			So(leader.Metrics[types.MetricKey(types.MetricLeaderEvidence, types.SideNone)].Raw, ShouldBeGreaterThan, 0)
			So(leader.Metrics[types.MetricKey(types.MetricDivergentScore, types.SideNone)].Raw, ShouldBeGreaterThan, 0)
			So(leader.Sample(types.MetricChange, types.SideNone).Normalized,
				ShouldNotBeNil)
			So(*leader.Sample(types.MetricBreadth, types.SideNone).Normalized,
				ShouldAlmostEqual, -1.0/3.0, 1e-12)
		})

		Convey("It should reject repeated latest-value cache entries", func() {
			So(signal.Measure(thesis), ShouldBeEmpty)
		})
	})

	Convey("Given a cohort with real cadence but no return dispersion", t, func() {
		signal := &Signal{observations: make(map[string]returnObservation)}
		thesis := types.NewThesis(nil)
		start := time.Unix(1_700_000_100, 0).UTC()
		thesis.Tickers = tickerMap([]kraken.TickerData{
			ticker("AAA/USD", 100, start),
			ticker("BBB/USD", 100, start),
		})
		So(signal.Measure(thesis), ShouldBeEmpty)
		thesis.Tickers = tickerMap([]kraken.TickerData{
			ticker("AAA/USD", 100, start.Add(time.Second)),
			ticker("BBB/USD", 100, start.Add(time.Second)),
		})
		measurements := signal.Measure(thesis)

		Convey("It should not fabricate normalized zeroes without a scale", func() {
			So(measurements, ShouldHaveLength, 2)
			So(measurements[0].Validity.State, ShouldEqual, types.ValidityProvisional)
			So(measurements[0].Sample(types.MetricChange, types.SideNone).Normalized,
				ShouldBeNil)
		})
	})
}

func tickerMap(rows []kraken.TickerData) *sync.Map {
	values := &sync.Map{}

	for _, row := range rows {
		values.Store(row.Symbol, row)
	}

	return values
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
