package sentiment

import (
	"context"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/qpool"
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
	Convey("Given a sentiment signal with a bullish cross-section", t, func() {
		ctx := context.Background()
		pool := qpool.NewQ(ctx, 2, 4, qpool.NewConfig())
		defer pool.Close()

		signal := NewSignal(ctx, pool)
		defer signal.Close()

		signal.symbols.Store("A/EUR", 2.0)
		signal.symbols.Store("B/EUR", 3.0)
		signal.symbols.Store("C/EUR", 1.5)

		Convey("When breadth clears the risk-on threshold", func() {
			measurement, ok := signal.measure(2.0)

			Convey("It should classify a risk-on surge", func() {
				So(ok, ShouldBeTrue)
				So(measurement.Source, ShouldEqual, perspectives.SourceSentiment)
				So(measurement.Category, ShouldEqual, perspectives.CategoryRiskOnSurge)
				So(measurement.SNR, ShouldBeGreaterThanOrEqualTo, 1)
			})
		})
	})

	Convey("Given a weak cross-section with a local leader", t, func() {
		signal := &Signal{}
		signal.symbols.Store("LEAD/EUR", 4.0)
		signal.symbols.Store("LAG/EUR", -2.0)
		signal.symbols.Store("FLAT/EUR", -1.0)

		Convey("When the symbol leads the pack", func() {
			measurement, ok := signal.measure(4.0)

			Convey("It should classify a divergent move", func() {
				So(ok, ShouldBeTrue)
				So(measurement.Category, ShouldEqual, perspectives.CategoryDivergentMove)
			})
		})
	})
}

func TestCategory(t *testing.T) {
	Convey("Given a sentiment signal", t, func() {
		signal := &Signal{}

		Convey("It should map breadth and leadership onto sentiment categories", func() {
			So(signal.category(0.60, 1.0, 2.0), ShouldEqual, perspectives.CategoryRiskOnSurge)
			So(signal.category(0.40, 2.0, 4.0), ShouldEqual, perspectives.CategoryDivergentMove)
			So(signal.category(0.40, -0.5, 2.0), ShouldEqual, perspectives.CategorySystemicSlump)
		})
	})
}

func BenchmarkMeasure(b *testing.B) {
	ctx := context.Background()
	pool := qpool.NewQ(ctx, 2, 4, qpool.NewConfig())
	defer pool.Close()

	signal := NewSignal(ctx, pool)
	defer signal.Close()

	signal.symbols.Store("A/EUR", 2.0)
	signal.symbols.Store("B/EUR", 3.0)
	signal.symbols.Store("C/EUR", 1.5)

	b.ReportAllocs()

	for b.Loop() {
		_, _ = signal.measure(2.0)
	}
}
