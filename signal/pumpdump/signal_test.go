package pumpdump

import (
	"testing"
	"time"

	spotbook "github.com/krakenfx/api-go/v2/pkg/book"
	"github.com/krakenfx/api-go/v2/pkg/decimal"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/spf13/viper"
	"github.com/theapemachine/nomagique/equation"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/types"
)

func TestSeenTrade(t *testing.T) {
	Convey("Given an exact-once cursor for one pumpdump symbol", t, func() {
		signal := &Signal{lastTrade: make(map[string]tradeCursor)}
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
		signal := &Signal{lastTrade: make(map[string]tradeCursor)}
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
		signal := &Signal{
			algo:      equation.NewIgnition(128),
			lastTrade: make(map[string]tradeCursor),
		}
		thesis := types.NewThesis()
		base := time.Unix(1_700_002_200, 0).UTC()
		var measurements []*types.Measurement

		for index, price := range []float64{100, 101, 100, 102, 101, 104} {
			at := base.Add(time.Duration(index) * time.Second)
			thesis.Books.Store("BTC/USD", pumpdumpBook("BTC/USD", price-0.5, price+0.5, at))
			thesis.Trades.Store(index, pumpdumpTrade(int64(index+1), price, at))
			measurements = signal.Measure(thesis)
			thesis.Trades.Delete(index)
		}

		Convey("It preserves legacy keys and publishes both dimensionless directional families", func() {
			So(measurements, ShouldHaveLength, 1)
			measurement := measurements[0]
			So(measurement.Metrics, ShouldHaveLength, 20)
			So(measurement.Sample(types.MetricRVOL, types.SideNone).Unit,
				ShouldEqual, types.UnitDimensionless)
			So(measurement.Sample(types.MetricSpread, types.SideNone).Unit,
				ShouldEqual, types.UnitQuoteCurrency)
			So(measurement.Sample(types.MetricPrecursor, types.SideBuy).Raw,
				ShouldBeGreaterThan, 0)
			So(measurement.Sample(types.MetricPrecursor, types.SideSell).Raw,
				ShouldEqual, 0)

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
			}
		})
	})
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
