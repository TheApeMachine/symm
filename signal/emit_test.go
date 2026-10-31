package signal

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/types"
)

func TestEmit(t *testing.T) {
	Convey("Emit addresses hub frames by signal origin and symbol scope", t, func() {
		prev := types.Focus()
		Reset(func() { types.SetFocus(prev) })

		emit := NewEmit()
		types.SetFocus("BTC/USD")

		Convey("wireAddress", func() {
			origin, scope, rest := emit.wireAddress([]string{
				"cvd", "BTC/USD", "aggressor_backing", "cvd_divergence",
			})
			So(origin, ShouldEqual, "cvd")
			So(scope, ShouldEqual, "BTC/USD")
			So(rest, ShouldResemble, []string{"aggressor_backing", "cvd_divergence"})

			origin, scope, rest = emit.wireAddress([]string{
				"liquidity", "aggregate", "market_friction",
			})
			So(origin, ShouldEqual, "liquidity")
			So(scope, ShouldEqual, "aggregate")
			So(rest, ShouldResemble, []string{"market_friction"})
		})

		Convey("allows focus and aggregate only", func() {
			So(emit.allows([]string{"cvd", "BTC/USD", "x"}), ShouldBeTrue)
			So(emit.allows([]string{"cvd", "ETH/USD", "x"}), ShouldBeFalse)
			So(emit.allows([]string{"liquidity", "aggregate", "x"}), ShouldBeTrue)

			types.SetFocus("")
			So(emit.allows([]string{"cvd", "BTC/USD", "x"}), ShouldBeFalse)
			So(emit.allows([]string{"liquidity", "aggregate", "x"}), ShouldBeTrue)
		})
	})
}
