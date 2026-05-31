package liquidity

import (
	"context"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/kraken/market"
	"github.com/theapemachine/symm/market/perspectives"
)

func TestNewSignal(t *testing.T) {
	Convey("Given a qpool", t, func() {
		ctx := context.Background()
		pool := qpool.NewQ(ctx, 2, 4, qpool.NewConfig())
		defer pool.Close()

		signal := NewSignal(ctx, pool)
		defer signal.Close()

		Convey("It should expose a measurements broadcast", func() {
			So(signal.broadcasts["measurements"], ShouldNotBeNil)
		})
	})
}

func TestMeasure(t *testing.T) {
	Convey("Given a liquidity signal with a seeded cross-section", t, func() {
		ctx := context.Background()
		pool := qpool.NewQ(ctx, 2, 4, qpool.NewConfig())
		defer pool.Close()

		signal := NewSignal(ctx, pool)
		defer signal.Close()

		signal.symbols.Store("ALT/EUR", 1250.0)
		signal.symbols.Store("COIN/EUR", 800.0)
		signal.symbols.Store("PEER/EUR", 900.0)

		Convey("When the symbol sits well above the peer median", func() {
			measurement, ok := signal.measure(market.TickerUpdate{
				Symbol: "ALT/EUR",
				Last:   10,
				Volume: 125,
			})

			Convey("It should publish robust liquidity", func() {
				So(ok, ShouldBeTrue)
				So(measurement.Source, ShouldEqual, perspectives.SourceLiquidity)
				So(measurement.Category, ShouldEqual, perspectives.CategoryRobustLiquidity)
				So(measurement.SNR, ShouldBeGreaterThanOrEqualTo, 0)
			})
		})

		Convey("When fewer than two peers exist", func() {
			lone := NewSignal(ctx, pool)
			defer lone.Close()
			lone.symbols.Store("SOLO/EUR", 500.0)

			_, ok := lone.measure(market.TickerUpdate{
				Symbol: "SOLO/EUR",
				Last:   5,
				Volume: 100,
			})

			Convey("It should withhold the reading", func() {
				So(ok, ShouldBeFalse)
			})
		})
	})
}

func TestCategory(t *testing.T) {
	Convey("Given a liquidity signal", t, func() {
		signal := &Signal{}

		Convey("It should map ratio bands onto scarcity categories", func() {
			So(signal.category(1.30), ShouldEqual, perspectives.CategoryRobustLiquidity)
			So(signal.category(1.00), ShouldEqual, perspectives.CategoryMedianDepth)
			So(signal.category(0.50), ShouldEqual, perspectives.CategoryExtremeScarcity)
		})
	})
}

func BenchmarkMeasure(b *testing.B) {
	ctx := context.Background()
	pool := qpool.NewQ(ctx, 2, 4, qpool.NewConfig())
	defer pool.Close()

	signal := NewSignal(ctx, pool)
	defer signal.Close()

	signal.symbols.Store("ALT/EUR", 1250.0)
	signal.symbols.Store("COIN/EUR", 800.0)
	signal.symbols.Store("PEER/EUR", 900.0)

	row := market.TickerUpdate{Symbol: "ALT/EUR", Last: 10, Volume: 125}

	b.ReportAllocs()

	for b.Loop() {
		_, _ = signal.measure(row)
	}
}
