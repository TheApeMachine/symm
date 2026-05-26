package liquidity

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestLiquidityConfidenceSingleCandidate(t *testing.T) {
	Convey("Given one illiquid symbol without peer context", t, func() {
		liquidity := &Liquidity{}
		confidence := liquidity.confidenceFromScore(0.85, nil)

		Convey("It should reflect the score without pinning at fifty percent", func() {
			So(confidence, ShouldBeGreaterThan, 0.5)
			So(confidence, ShouldBeLessThan, 1)
		})
	})
}

func TestLiquidityConfidencePeerLead(t *testing.T) {
	Convey("Given a symbol leading peers on illiquidity", t, func() {
		liquidity := &Liquidity{}
		confidence := liquidity.confidenceFromScore(0.8, []float64{0.3, 0.4})

		Convey("It should combine depth and lead", func() {
			So(confidence, ShouldBeGreaterThan, 0)
			So(confidence, ShouldBeLessThan, 1)
		})
	})
}
