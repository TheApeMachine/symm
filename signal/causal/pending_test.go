package causal

import (
	"testing"
	"time"

	"github.com/theapemachine/symm/engine"
	"github.com/theapemachine/symm/kraken/asset"

	. "github.com/smartystreets/goconvey/convey"
)

func TestCausalSymbolResolvePendingLocked(t *testing.T) {
	Convey("Given a pending feature reading with a matured forward window", t, func() {
		state := NewCausalSymbol(asset.Pair{Wsname: "BTC/EUR"}, engine.DefaultCalibrationParams())
		start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

		state.lastPrice = 100
		state.enqueuePendingLocked(0.1, 1, 0.5, 100, start)

		state.lastPrice = 110
		state.resolvePendingLocked(start.Add(causalForwardWindow + time.Nanosecond))

		Convey("It should label the sample with realized forward return", func() {
			So(state.pendingSamples, ShouldHaveLength, 0)
			So(state.samples, ShouldHaveLength, 1)
			So(state.samples[0].value(priceVelocityNode), ShouldAlmostEqual, 0.1, 1e-9)
		})
	})
}
