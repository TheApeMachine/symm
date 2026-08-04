package liquidity

import (
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
		signal := &Signal{observations: make(map[string]liquidityObservation)}
		thesis := types.NewThesis()
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

		thesis.Tickers = tickerMap(firstLeg)
		So(signal.Measure(thesis), ShouldHaveLength, 4)
		thesis.Tickers = tickerMap(secondLeg)
		measurements := signal.Measure(thesis)

		Convey("It should use a leave-one-out robust baseline", func() {
			thin := measurementFor(measurements, "THIN/USD")
			So(thin, ShouldNotBeNil)
			depth := thin.Metrics[types.MetricKey(types.MetricExecutableTouchDepth, types.SideNone)]
			median := thin.Metrics[types.MetricKey(types.MetricExecutableTouchDepthMedian, types.SideNone)].Raw
			relative := thin.Metrics[types.MetricKey(types.MetricRelativeTouchDepth, types.SideNone)].Raw
			scarcity := thin.Metrics[types.MetricKey(types.MetricScarcityScore, types.SideNone)].Raw
			So(depth.Raw, ShouldAlmostEqual, 101)
			So(median, ShouldAlmostEqual, 1212)
			So(relative, ShouldAlmostEqual, 101.0/1212.0, 1e-12)
			So(depth.Normalized, ShouldNotBeNil)
			So(*depth.Normalized, ShouldAlmostEqual, relative, 1e-12)
			So(scarcity, ShouldAlmostEqual, 1111.0/(1111.0+202.0), 1e-12)
		})

		Convey("It should not emit unchanged cached observations", func() {
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
