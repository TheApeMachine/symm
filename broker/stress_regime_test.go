package broker

import (
	"testing"
	"time"

	"github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/market/perspectives"
)

func TestStressRegimeFrom(t *testing.T) {
	convey.Convey("Given fluid gauge factors", t, func() {
		regime := StressRegimeFrom([]perspectives.Measurement{{
			Source: perspectives.SourceFluid,
			Factors: []perspectives.GaugeFactor{
				{Name: "turb_fd", Value: 9},
				{Name: "vort", Value: -0.4},
			},
		}})

		convey.Convey("It should derive turbulence and vorticity intensity", func() {
			convey.So(regime.Turbulence, convey.ShouldEqual, 9)
			convey.So(regime.Vorticity, convey.ShouldEqual, 0.4)
			convey.So(regime.Multiplier(), convey.ShouldEqual, 10)
		})
	})
}

func TestEffectiveRejectRate(t *testing.T) {
	convey.Convey("Given a turbulent micro-regime", t, func() {
		rate := EffectiveRejectRate(0.05, StressRegime{Turbulence: 9})

		convey.Convey("It should scale the baseline reject rate", func() {
			convey.So(rate, convey.ShouldEqual, 0.5)
		})
	})
}

func TestEffectiveStressLatency(t *testing.T) {
	convey.Convey("Given elevated vorticity", t, func() {
		latency := EffectiveStressLatency(50*time.Millisecond, StressRegime{Vorticity: 4})

		convey.Convey("It should swell quote-age stress", func() {
			convey.So(latency, convey.ShouldEqual, 250*time.Millisecond)
		})
	})
}

func BenchmarkStressRegimeFrom(b *testing.B) {
	measurements := []perspectives.Measurement{{
		Source: perspectives.SourceFluid,
		Factors: []perspectives.GaugeFactor{
			{Name: "turb_fd", Value: 2},
			{Name: "vort", Value: 0.5},
		},
	}}

	b.ReportAllocs()

	for b.Loop() {
		_ = StressRegimeFrom(measurements)
	}
}
