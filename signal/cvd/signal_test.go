package cvd

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	krakenmarket "github.com/theapemachine/symm/kraken/market"
	"github.com/theapemachine/symm/logic"
)

func TestSignalMeasure(t *testing.T) {
	Convey("Given aggressive buy flow with rising price", t, func() {
		signal := NewSignal(
			"BTC/EUR",
			logic.NewEntity(logic.EntityTrade),
			8,
			2.0,
			0.5,
		)

		for _, price := range []float64{100, 101, 102, 103, 104} {
			signal.Record(&krakenmarket.TradeUpdate{
				Symbol: "BTC/EUR",
				Side:   "buy",
				Price:  price,
				Qty:    1,
			})
		}

		measurement, err := signal.Measure(nil)

		Convey("It should classify aggressive drive", func() {
			So(err, ShouldBeNil)
			So(measurement.Source, ShouldEqual, logic.SourceCVD)
			So(measurement.Category, ShouldEqual, logic.CategoryAggressiveDrive)
			So(measurement.Strength, ShouldBeGreaterThan, 0)
			So(measurement.Confidence, ShouldBeGreaterThan, 0)
		})
	})

	Convey("Given aggressive buy flow with flat price", t, func() {
		signal := NewSignal(
			"ETH/EUR",
			logic.NewEntity(logic.EntityTrade),
			8,
			2.0,
			0.5,
		)

		for range 4 {
			signal.Record(&krakenmarket.TradeUpdate{
				Symbol: "ETH/EUR",
				Side:   "buy",
				Price:  50,
				Qty:    2,
			})
		}

		measurement, err := signal.Measure(nil)

		Convey("It should classify hidden absorption", func() {
			So(err, ShouldBeNil)
			So(measurement.Category, ShouldEqual, logic.CategoryHiddenAbsorption)
		})
	})

	Convey("Given balanced two-sided flow", t, func() {
		signal := NewSignal(
			"SOL/EUR",
			logic.NewEntity(logic.EntityTrade),
			8,
			2.0,
			0.5,
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

		for _, trade := range trades {
			signal.Record(&krakenmarket.TradeUpdate{
				Symbol: "SOL/EUR",
				Side:   trade.side,
				Price:  trade.price,
				Qty:    1,
			})
		}

		measurement, err := signal.Measure(nil)

		Convey("It should classify stochastic balance", func() {
			So(err, ShouldBeNil)
			So(measurement.Category, ShouldEqual, logic.CategoryStochasticBalance)
		})
	})

	Convey("Given insufficient trades", t, func() {
		signal := NewSignal(
			"XRP/EUR",
			logic.NewEntity(logic.EntityTrade),
			4,
			2.0,
			0.5,
		)

		measurement, err := signal.Measure(nil)

		Convey("It should classify volume starvation", func() {
			So(err, ShouldBeNil)
			So(measurement.Category, ShouldEqual, logic.CategoryVolumeStarvation)
		})
	})
}

func BenchmarkSignalMeasure(b *testing.B) {
	signal := NewSignal(
		"BTC/EUR",
		logic.NewEntity(logic.EntityTrade),
		64,
		2.0,
		0.5,
	)

	for index := 0; index < 64; index++ {
		signal.Record(&krakenmarket.TradeUpdate{
			Symbol: "BTC/EUR",
			Side:   "buy",
			Price:  100 + float64(index)*0.1,
			Qty:    1,
		})
	}

	b.ResetTimer()

	for b.Loop() {
		_, _ = signal.Measure(nil)
	}
}
