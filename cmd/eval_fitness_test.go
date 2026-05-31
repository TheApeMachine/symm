package cmd

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/config"
	"github.com/theapemachine/symm/trader/economics"
)

func TestTuneFitness(t *testing.T) {
	Convey("Given wallet score and missed gate regret", t, func() {
		fitness := TuneFitness(12, 3, economics.PerformanceSummary{})

		Convey("It should subtract missed forward EUR from score", func() {
			So(fitness, ShouldEqual, 9)
		})
	})

	Convey("Given zero missed regret", t, func() {
		fitness := TuneFitness(-4, 0, economics.PerformanceSummary{})

		Convey("It should equal score alone", func() {
			So(fitness, ShouldEqual, -4)
		})
	})

	Convey("Given equal profitable score with different hold times", t, func() {
		previousTTL := config.System.PerspectiveTTL
		config.System.PerspectiveTTL = time.Second
		t.Cleanup(func() { config.System.PerspectiveTTL = previousTTL })

		fast := TuneFitness(10, 0, economics.PerformanceSummary{
			ProfitableTrades: 1,
			MeanProfitHoldMS: 100,
		})
		slow := TuneFitness(10, 0, economics.PerformanceSummary{
			ProfitableTrades: 1,
			MeanProfitHoldMS: 900,
		})

		Convey("It should prefer faster time to profit", func() {
			So(fast, ShouldBeGreaterThan, slow)
		})
	})
}

func BenchmarkTuneFitness(b *testing.B) {
	performance := economics.PerformanceSummary{
		ProfitableTrades: 3,
		MeanProfitHoldMS: 750,
	}

	for b.Loop() {
		_ = TuneFitness(12.5, 1.2, performance)
	}
}
