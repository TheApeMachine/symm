package fluid

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/engine"
	"github.com/theapemachine/symm/kraken/asset"
	"github.com/theapemachine/symm/kraken/market"
)

func TestFluidSymbolApplyFeedback(t *testing.T) {
	Convey("Given a fluid symbol forecast learner", t, func() {
		state := NewFluidSymbol(asset.Pair{Wsname: "ALT/EUR"})
		before := state.forecast.Scale()

		state.ApplyFeedback(engine.PredictionFeedback{
			Source:          fluidSource,
			Symbol:          "ALT/EUR",
			PredictedReturn: 0.03,
			ActualReturn:    -0.02,
		})

		Convey("It should fold realized error into the forecast scale", func() {
			So(state.forecast.Scale(), ShouldBeLessThan, before)
		})
	})
}

func TestFluidSymbolFeedTickerVolumeTarget(t *testing.T) {
	Convey("Given a ticker with daily volume", t, func() {
		state := NewFluidSymbol(asset.Pair{Wsname: "ALT/EUR"})

		state.FeedTicker(market.TickerRow{
			Last:      10,
			Volume:    8640,
			ChangePct: 1.5,
		})

		Convey("It should set the volume-clocked bar target", func() {
			So(state.flux.target, ShouldBeGreaterThan, 0)
		})
	})
}

func TestFluidSymbolWireRowTurbulence(t *testing.T) {
	Convey("Given repeated price observations", t, func() {
		state := NewFluidSymbol(asset.Pair{Wsname: "ALT/EUR"})
		state.volume = 1000
		state.changePct = 0.5

		for index := range 32 {
			state.FeedTicker(market.TickerRow{
				Last:   10 + float64(index)*0.05,
				Volume: 1000,
			})
		}

		row := state.wireRow()

		Convey("It should expose fractional-diff turbulence on the wire row", func() {
			So(row, ShouldNotBeNil)
			So(row["fd_ret"], ShouldNotBeNil)
			So(row["re"], ShouldBeGreaterThan, 0)
		})
	})
}

func BenchmarkFluidSymbolApplyFeedback(b *testing.B) {
	state := NewFluidSymbol(asset.Pair{Wsname: "ALT/EUR"})
	feedback := engine.PredictionFeedback{
		PredictedReturn: 0.02,
		ActualReturn:    -0.01,
	}

	for b.Loop() {
		state.ApplyFeedback(feedback)
	}
}
