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
		thesis := types.NewThesis()
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
		})

		Convey("It should reject repeated latest-value cache entries", func() {
			So(signal.Measure(thesis), ShouldBeEmpty)
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
