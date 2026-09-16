package types

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/data"
)

func TestAllowsRoute(t *testing.T) {
	Convey("Given dashboard route gating", t, func() {
		originalRoute := Route()
		originalFocus := Focus()

		Reset(func() {
			SetRoute(originalRoute)
			SetFocus(originalFocus)
		})

		liquidity := data.NewMeasurement[float64]("liquidity", nil)
		liquidity.Label = "BTC/USD"
		resonance := data.NewMeasurement[float64]("resonance", nil)
		resonance.Label = "BTC/USD"
		manifold := data.NewMeasurement[float64]("manifold", nil)
		manifold.Label = "BTC/USD"
		unlabeled := data.NewMeasurement[float64]("liquidity", nil)
		eth := data.NewMeasurement[float64]("hawkes", nil)
		eth.Label = "ETH/USD"

		Convey("dashboard publishes the focused signal while resonance uses WebRTC", func() {
			SetRoute("dashboard")
			SetFocus("BTC/USD")

			So(AllowsRoute(liquidity), ShouldBeTrue)
			So(AllowsRoute(resonance), ShouldBeFalse)
			So(RouteDropReason(resonance), ShouldEqual, "webrtc")
			So(AllowsRoute(eth), ShouldBeFalse)
			So(RouteDropReason(eth), ShouldEqual, "focus")
			So(AllowsRoute(manifold), ShouldBeFalse)
			So(AllowsRoute(unlabeled), ShouldBeFalse)
			So(RouteDropReason(unlabeled), ShouldEqual, "empty-label")
		})

		Convey("no page sends a scalar resonance packet over the full WebRTC result", func() {
			for _, route := range []string{"dashboard", "xray", "signals", "cortex"} {
				SetRoute(route)
				So(AllowsRoute(resonance), ShouldBeFalse)
			}
		})

		Convey("colon-suffixed kernel sources still count as signals", func() {
			SetRoute("dashboard")
			SetFocus("BTC/USD")

			level3 := data.NewMeasurement[float64]("pumpdump:level3", nil)
			level3.Label = "BTC/USD"
			So(AllowsRoute(level3), ShouldBeTrue)
			So(RouteDropReason(level3), ShouldEqual, "")

			other := data.NewMeasurement[float64]("toxicity:level3", nil)
			other.Label = "ZEC/USD"
			So(AllowsRoute(other), ShouldBeFalse)
			So(RouteDropReason(other), ShouldEqual, "focus")
		})

		Convey("fluid publishes manifold measurements", func() {
			SetRoute("fluid")

			So(AllowsRoute(manifold), ShouldBeTrue)
			So(AllowsRoute(liquidity), ShouldBeFalse)
			So(AllowsRoute(resonance), ShouldBeFalse)
		})
	})
}

func TestAllowsWebRTC(t *testing.T) {
	Convey("Routes select structured result sources and symbol scope", t, func() {
		originalRoute, originalFocus := Route(), Focus()
		defer SetRoute(originalRoute)
		defer SetFocus(originalFocus)
		SetFocus("BTC/USD")

		for _, route := range []string{"dashboard", "xray", "fluid", "learning", "signals", "cortex", "journal", "diagnostics"} {
			SetRoute(route)
			So(AllowsWebRTC("manifold", ""), ShouldEqual, route == "fluid")
			So(AllowsWebRTC("resonance", "BTC/USD"), ShouldEqual, route == "dashboard" || route == "xray")
			So(AllowsWebRTC("resonance:solver", "ETH/USD"), ShouldEqual, route == "xray")
			So(AllowsWebRTC("resonance", ""), ShouldBeFalse)
			So(AllowsWebRTC("public", "BTC/USD"), ShouldBeFalse)
		}
	})
}
