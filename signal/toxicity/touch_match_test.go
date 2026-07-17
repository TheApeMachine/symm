package toxicity

import (
	"testing"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	. "github.com/smartystreets/goconvey/convey"
)

func TestTouchMatchWithinTick(t *testing.T) {
	Convey("Given a trade and touch price separated by one tick", t, func() {
		tradePrice := *decimal.NewFromFloat64(100)
		touchPrice := decimal.NewFromFloat64(100.0001)
		increment := decimal.NewFromFloat64(0.0001)

		Convey("Then touchMatch should accept it as a fill", func() {
			So(touchMatch(tradePrice, touchPrice, increment), ShouldBeTrue)
		})
	})
}

func TestTouchMatchRejectsBeyondTick(t *testing.T) {
	Convey("Given a trade further than one tick from the touch", t, func() {
		tradePrice := *decimal.NewFromFloat64(100)
		touchPrice := decimal.NewFromFloat64(100.0003)
		increment := decimal.NewFromFloat64(0.0001)

		Convey("Then touchMatch should reject it", func() {
			So(touchMatch(tradePrice, touchPrice, increment), ShouldBeFalse)
		})
	})
}

func TestTouchMatchFallsBackToExactWhenIncrementMissing(t *testing.T) {
	Convey("Given no usable price increment", t, func() {
		tradePrice := *decimal.NewFromFloat64(100)
		increment := decimal.NewFromFloat64(0)

		Convey("Then touchMatch should fall back to exact equality", func() {
			So(touchMatch(tradePrice, decimal.NewFromFloat64(100), increment), ShouldBeTrue)
			So(touchMatch(tradePrice, decimal.NewFromFloat64(100.0001), increment), ShouldBeFalse)
		})
	})
}
