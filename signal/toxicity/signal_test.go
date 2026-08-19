package toxicity

import (
	"context"
	"slices"
	"testing"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/types"
)

func TestMeasure(t *testing.T) {
	Convey("Given an empty Level 3 update", t, func() {
		signal := NewSignal(context.Background(), nil)
		market := types.NewSymbol("BTC/USD", nil)
		base := time.Unix(1_700_004_000, 0).UTC()
		market.AppendLevel3(kraken.Level3Data{
			Symbol: "BTC/USD", Type: "update", Timestamp: base,
		})

		Convey("It should emit an immature zero reading instead of going dark", func() {
			err := signal.Measure(market)
			So(err, ShouldBeNil)
			measurements := slices.Collect(market.MarketMeasurements("category"))
			So(measurements, ShouldHaveLength, 1)
			So(measurements[0].Source, ShouldEqual, types.SourceToxicity)
			So(measurements[0].Maturity, ShouldEqual, 0)
			So(measurements[0].Metrics, ShouldBeEmpty)
		})

		market.AppendLevel3(toxicityLevel3(
			"snapshot", 10, 10, base.Add(time.Second),
		))

		Convey("It should classify once both book sides establish a real midpoint", func() {
			err := signal.Measure(market)
			So(err, ShouldBeNil)
			measurements := slices.Collect(market.MarketMeasurements("category"))
			So(measurements, ShouldHaveLength, 2)
			So(measurements[1].Sample(types.MetricMidpoint, types.SideNone).Raw,
				ShouldEqual, 100.5)
		})
	})

	Convey("Given toxicity book observations on one symbol", t, func() {
		signal := NewSignal(context.Background(), nil)
		market := types.NewSymbol("BTC/USD", nil)
		base := time.Unix(1_700_004_100, 0).UTC()
		market.AppendLevel3(toxicityLevel3("snapshot", 10, 10, base))

		err := signal.Measure(market)
		So(err, ShouldBeNil)
		measurements := slices.Collect(market.MarketMeasurements("category"))

		Convey("It should emit classified book-quality metrics from nomagique", func() {
			So(measurements, ShouldHaveLength, 1)
			measurement := measurements[0]
			So(measurement.Source, ShouldEqual, types.SourceToxicity)
			So(measurement.Symbol, ShouldEqual, "BTC/USD")
			So(measurement.Sample(types.MetricTouchQuantity, types.SideBuy).Raw, ShouldEqual, 10.0)
			So(measurement.Sample(types.MetricTouchQuantity, types.SideSell).Raw, ShouldEqual, 10.0)
			So(measurement.Sample(types.MetricBluffScore, types.SideNone).Normalized, ShouldNotBeNil)
			So(measurement.Sample(types.MetricVacuumScore, types.SideNone).Normalized, ShouldNotBeNil)
			So(measurement.Sample(types.MetricSupportScore, types.SideNone).Normalized, ShouldNotBeNil)
		})
	})

	Convey("Given a trade before its matching Level 3 delete", t, func() {
		signal := NewSignal(context.Background(), nil)
		market := types.NewSymbol("BTC/USD", nil)
		base := time.Unix(1_700_004_200, 0).UTC()
		market.AppendLevel3(toxicityLevel3("snapshot", 10, 10, base))
		err := signal.Measure(market)
		So(err, ShouldBeNil)
		So(slices.Collect(market.MarketMeasurements("category")), ShouldHaveLength, 1)
		market.AppendTrade(toxicityTrade(91, "sell", 100, 2, base.Add(time.Second)))
		market.AppendLevel3(toxicityDelete(
			"bid-order",
			100,
			10,
			base.Add(2*time.Second),
		))

		err := signal.Measure(market)
		So(err, ShouldBeNil)
		measurements := slices.Collect(market.MarketMeasurements("category"))

		Convey("It should corroborate the delete as a fill", func() {
			So(measurements, ShouldHaveLength, 1)
			So(measurements[0].Sample(types.MetricFillVolume, types.SideBuy).Raw,
				ShouldBeGreaterThan, 0.0)
		})
	})

	Convey("Given a changed touch with accounted cancellation flow", t, func() {
		signal := NewSignal(context.Background(), nil)
		market := types.NewSymbol("BTC/USD", nil)
		base := time.Unix(1_700_004_300, 0).UTC()
		market.AppendLevel3(toxicityLevel3("snapshot", 10, 10, base))
		err := signal.Measure(market)
		So(err, ShouldBeNil)
		So(slices.Collect(market.MarketMeasurements("category")), ShouldHaveLength, 1)
		market.AppendLevel3(toxicityDelete(
			"bid-order",
			100,
			10,
			base.Add(time.Second),
		))

		err := signal.Measure(market)
		So(err, ShouldBeNil)
		measurements := slices.Collect(market.MarketMeasurements("category"))
		So(measurements, ShouldHaveLength, 1)
		measurement := measurements[0]

		Convey("Then raw quantities remain intact and HypothesisSeparation uses dimensionless shares", func() {
			cancelled := measurement.Sample(types.MetricCancelledQuantity, types.SideBuy)
			So(cancelled.Raw, ShouldBeGreaterThan, 0)
			So(cancelled.Normalized, ShouldNotBeNil)
			So(*cancelled.Normalized, ShouldEqual, 1.0)
			separation := measurement.Sample(types.MetricHypothesisSeparation, types.SideNone)
			So(separation.Normalized, ShouldNotBeNil)
			So(*separation.Normalized, ShouldEqual, 1.0)
		})
	})
}

func toxicityTrade(
	id int64,
	side string,
	price float64,
	quantity float64,
	at time.Time,
) kraken.TradeData {
	return kraken.TradeData{
		Symbol: "BTC/USD", Side: side, Price: *decimal.NewFromFloat64(price),
		Qty: quantity, TradeID: id, Timestamp: at,
	}
}

func toxicityLevel3(
	frameType string,
	bidQuantity float64,
	askQuantity float64,
	at time.Time,
) kraken.Level3Data {
	return kraken.Level3Data{
		Symbol:    "BTC/USD",
		Type:      frameType,
		Timestamp: at,
		Bids: []kraken.Level3Order{{
			Event: "add", OrderID: "bid-order",
			LimitPrice: decimal.NewFromFloat64(100),
			OrderQty:   decimal.NewFromFloat64(bidQuantity), Timestamp: at,
		}},
		Asks: []kraken.Level3Order{{
			Event: "add", OrderID: "ask-order",
			LimitPrice: decimal.NewFromFloat64(101),
			OrderQty:   decimal.NewFromFloat64(askQuantity), Timestamp: at,
		}},
	}
}

func toxicityDelete(
	orderID string,
	price float64,
	quantity float64,
	at time.Time,
) kraken.Level3Data {
	return kraken.Level3Data{
		Symbol:    "BTC/USD",
		Type:      "update",
		Timestamp: at,
		Bids: []kraken.Level3Order{{
			Event: "delete", OrderID: orderID,
			LimitPrice: decimal.NewFromFloat64(price),
			OrderQty:   decimal.NewFromFloat64(quantity), Timestamp: at,
		}},
	}
}
