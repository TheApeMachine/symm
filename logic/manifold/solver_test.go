//go:build darwin && cgo

package manifold

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/spf13/viper"
	"github.com/theapemachine/nomagique/algorithm/excitation"
	"github.com/theapemachine/nomagique/hawkes"
)

func TestSolverAdvance(t *testing.T) {
	viper.Set("market.l3_depth", 8)
	viper.Set("signals.fluid.integration_interval", 100*time.Millisecond)
	viper.Set("market.manifold_max_symbols", 8)

	Convey("Given Hawkes excitation", t, func() {
		solver, err := NewSolver(newTestBookSource("BTC/USD"))
		So(err, ShouldBeNil)
		defer solver.Close()

		at := time.Unix(1, 0)
		state, advanceErr := solver.advance(
			"BTC/USD",
			solverOutcome(at, 4, 2),
		)
		stale, staleErr := solver.advance(
			"BTC/USD",
			solverOutcome(at, 4, 2),
		)

		Convey("It should return a finite GPU readout", func() {
			So(advanceErr, ShouldBeNil)
			So(state.GasReady(), ShouldBeTrue)
			So(state.Epoch, ShouldEqual, 1)
			So(state.Source, ShouldEqual, "manifold")
			So(state.Reading.CoherenceMag2, ShouldBeGreaterThan, 0)
			So(len(state.Rho), ShouldBeGreaterThan, 0)
			So(len(state.PsiMag2), ShouldBeGreaterThan, 0)
			So(len(state.Particles), ShouldBeGreaterThan, 0)
			So(staleErr, ShouldBeNil)
			So(stale.GasReady(), ShouldBeFalse)
		})
	})
}

func solverOutcome(at time.Time, buyRate, sellRate float64) excitation.Outcome {
	return excitation.Outcome{
		At:              at,
		Horizon:         time.Second,
		EventCount:      8,
		BuyArrivalRate:  buyRate,
		SellArrivalRate: sellRate,
		Maturity:        0.75,
		Readiness: excitation.Readiness{
			Observation: true,
			Intensity:   true,
			HawkesFit:   true,
		},
		Fit: hawkes.BivariateFit{
			MuX:            buyRate,
			MuY:            sellRate,
			AlphaXX:        buyRate,
			AlphaYY:        sellRate,
			AlphaXY:        buyRate * 0.1,
			AlphaYX:        sellRate * 0.1,
			Beta:           2,
			IntensityX:     buyRate,
			IntensityY:     sellRate,
			SpectralRadius: 0.35,
		},
	}
}

func BenchmarkSolverAdvance(b *testing.B) {
	viper.Set("market.l3_depth", 8)
	viper.Set("signals.fluid.integration_interval", 100*time.Millisecond)
	viper.Set("market.manifold_max_symbols", 8)

	solver, err := NewSolver(newTestBookSource("BTC/USD"))

	if err != nil {
		b.Fatal(err)
	}

	defer solver.Close()

	at := time.Unix(1, 0)
	b.ReportAllocs()

	for b.Loop() {
		outcome := solverOutcome(at, 4, 2)

		if _, advanceErr := solver.advance("BTC/USD", outcome); advanceErr != nil {
			b.Fatal(advanceErr)
		}

		at = at.Add(time.Nanosecond)
	}
}
