package cvd

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/spf13/viper"
	krakenmarket "github.com/theapemachine/symm/kraken/market"
	"github.com/theapemachine/symm/logic"
)

func setCVDTestConfig() {
	viper.Set("signals.cvd.measurements_capacity", 4)
}

func seedTrades(
	signal *Signal,
	symbol, side string,
	base time.Time,
	count int,
	startPrice float64,
) {
	for index := range count {
		signal.Record(&krakenmarket.TradeUpdate{
			Symbol:    symbol,
			Side:      side,
			Price:     startPrice + float64(index)*0.01,
			Qty:       1,
			Timestamp: base.Add(time.Duration(index) * time.Millisecond),
		})
	}
}

func TestSignalMeasure(t *testing.T) {
	setCVDTestConfig()
	eventAt := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	measureAt := eventAt.Add(time.Second)

	Convey("Given aggressive buy flow with rising price", t, func() {
		signal := NewSignal(
			"BTC/EUR",
			logic.NewEntity(logic.EntityTrade),
		)

		seedTrades(signal, "BTC/EUR", "buy", eventAt, 5, 100)

		measurement, err := signal.Measure(nil, measureAt)

		Convey("It should classify aggressive drive", func() {
			So(err, ShouldBeNil)
			So(measurement.Source, ShouldEqual, logic.SourceCVD)
			So(measurement.Category, ShouldEqual, logic.CategoryAggressiveDrive)
			So(measurement.Strength, ShouldBeGreaterThan, 0)
			So(measurement.Confidence, ShouldBeGreaterThan, 0)
			So(measurement.ObservedAt, ShouldEqual, measureAt)
		})
	})

	Convey("Given aggressive buy flow with flat price", t, func() {
		signal := NewSignal(
			"ETH/EUR",
			logic.NewEntity(logic.EntityTrade),
		)

		for index := range 4 {
			price := 50.0

			if index%2 == 1 {
				price = 50.001
			}

			signal.Record(&krakenmarket.TradeUpdate{
				Symbol:    "ETH/EUR",
				Side:      "buy",
				Price:     price,
				Qty:       2,
				Timestamp: eventAt.Add(time.Duration(index) * time.Millisecond),
			})
		}

		measurement, err := signal.Measure(nil, measureAt)

		Convey("It should classify hidden absorption", func() {
			So(err, ShouldBeNil)
			So(measurement.Category, ShouldEqual, logic.CategoryHiddenAbsorption)
			So(measurement.Confidence, ShouldBeGreaterThan, 0)
		})
	})

	Convey("Given balanced two-sided flow", t, func() {
		signal := NewSignal(
			"SOL/EUR",
			logic.NewEntity(logic.EntityTrade),
		)

		trades := []struct {
			side  string
			price float64
		}{
			{"buy", 25},
			{"sell", 25.1},
			{"buy", 25},
			{"sell", 25.1},
		}

		for index, trade := range trades {
			signal.Record(&krakenmarket.TradeUpdate{
				Symbol:    "SOL/EUR",
				Side:      trade.side,
				Price:     trade.price,
				Qty:       1,
				Timestamp: eventAt.Add(time.Duration(index) * time.Millisecond),
			})
		}

		measurement, err := signal.Measure(nil, measureAt)

		Convey("It should classify stochastic balance", func() {
			So(err, ShouldBeNil)
			So(measurement.Category, ShouldEqual, logic.CategoryStochasticBalance)
			So(measurement.Confidence, ShouldBeGreaterThan, 0)
		})
	})

	Convey("Given insufficient trades", t, func() {
		signal := NewSignal(
			"XRP/EUR",
			logic.NewEntity(logic.EntityTrade),
		)

		_, err := signal.Measure(nil, eventAt.Add(time.Second))

		Convey("It should withhold until trades are available", func() {
			So(err, ShouldBeNil)
		})
	})
}

func BenchmarkSignalMeasure(b *testing.B) {
	setCVDTestConfig()

	signal := NewSignal(
		"BTC/EUR",
		logic.NewEntity(logic.EntityTrade),
	)

	eventAt := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	seedTrades(signal, "BTC/EUR", "buy", eventAt, 64, 100)

	b.ResetTimer()

	for b.Loop() {
		_, _ = signal.Measure(nil, eventAt.Add(time.Second))
	}
}
