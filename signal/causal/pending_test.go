package causal

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
)

func TestResolvePendingLocked(t *testing.T) {
	Convey("Given pending samples past the forward window", t, func() {
		state := NewCausalSymbol()
		state.lastPrice = 110
		opened := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
		now := opened.Add(causalForwardWindow + time.Second)

		state.pendingSamples = []pendingCausalSample{{
			macroMomentum: 0.5,
			liquidity:     80,
			localFlow:     2,
			anchorPrice:   100,
			openedAt:      opened,
		}}

		state.resolvePendingLocked(now)

		Convey("It should label realized forward returns into training history", func() {
			So(len(state.samples), ShouldEqual, 1)
			So(state.samples[0].value(priceVelocityNode), ShouldAlmostEqual, 0.1, 1e-9)
			So(len(state.pendingSamples), ShouldEqual, 0)
		})
	})

	Convey("Given pending samples still inside the forward window", t, func() {
		state := NewCausalSymbol()
		state.lastPrice = 110
		opened := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)

		state.pendingSamples = []pendingCausalSample{{
			macroMomentum: 0.5,
			liquidity:     80,
			localFlow:     2,
			anchorPrice:   100,
			openedAt:      opened,
		}}

		state.resolvePendingLocked(opened.Add(causalForwardWindow / 2))

		Convey("It should keep them queued", func() {
			So(len(state.samples), ShouldEqual, 0)
			So(len(state.pendingSamples), ShouldEqual, 1)
		})
	})
}

func TestEnqueuePendingLocked(t *testing.T) {
	Convey("Given more pending samples than the cap", t, func() {
		state := NewCausalSymbol()
		now := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)

		for index := range causalPendingCap + 5 {
			state.enqueuePendingLocked(0.1, 10, 1, 100, now.Add(time.Duration(index)*time.Millisecond))
		}

		Convey("It should retain only the newest window", func() {
			So(len(state.pendingSamples), ShouldEqual, causalPendingCap)
		})
	})
}
