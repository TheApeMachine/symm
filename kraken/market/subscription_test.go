package market

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestValidBookDepth(t *testing.T) {
	Convey("Given arbitrary requested book depths", t, func() {
		Convey("It should snap a small or invalid depth up to the smallest Kraken depth", func() {
			So(validBookDepth(0), ShouldEqual, 10)
			So(validBookDepth(5), ShouldEqual, 10)
			So(validBookDepth(10), ShouldEqual, 10)
		})

		Convey("It should snap an in-between depth up to the next allowed depth", func() {
			So(validBookDepth(11), ShouldEqual, 25)
			So(validBookDepth(200), ShouldEqual, 500)
		})

		Convey("It should cap at the largest allowed depth", func() {
			So(validBookDepth(5000), ShouldEqual, 1000)
		})
	})
}

func TestClosed(t *testing.T) {
	Convey("Given a closed subscription channel", t, func() {
		ch := closed[TradeUpdate]()

		Convey("It should be already closed so a range exits immediately", func() {
			_, ok := <-ch
			So(ok, ShouldBeFalse)
		})
	})
}
