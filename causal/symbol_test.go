package causal

import (
	"testing"
	"time"

	"github.com/theapemachine/symm/engine"
	"github.com/theapemachine/symm/kraken/asset"

	. "github.com/smartystreets/goconvey/convey"
)

func TestCausalSymbolBuildSampleVelocity(t *testing.T) {
	Convey("Given two committed samples with a price move", t, func() {
		state := NewCausalSymbol(asset.Pair{Wsname: "BTC/EUR"}, engine.DefaultCalibrationParams())
		start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

		first, _ := state.buildSample(0.1, 1, 0.5, 100, start)
		state.commitSample(first, 100, start)

		second, _ := state.buildSample(
			0.1, 1, 0.5, 110, start.Add(10*time.Second),
		)

		Convey("It should use the prior committed price for velocity", func() {
			So(first.value(priceVelocityNode), ShouldEqual, 0)
			So(second.value(priceVelocityNode), ShouldAlmostEqual, 0.01, 1e-9)
		})
	})
}
