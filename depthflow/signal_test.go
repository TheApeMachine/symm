package depthflow

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/kraken/market"
)

func TestDepthImbalanceAtLevels(t *testing.T) {
	Convey("Given bid-heavy depth", t, func() {
		imbalance := depthImbalanceAtLevels(
			[]market.BookLevel{{Volume: 80}, {Volume: 20}},
			[]market.BookLevel{{Volume: 10}, {Volume: 10}},
		)

		Convey("It should return positive imbalance", func() {
			So(imbalance, ShouldBeGreaterThan, 0)
		})
	})
}

func TestLevelVolume(t *testing.T) {
	Convey("Given empty levels", t, func() {
		Convey("It should return zero volume", func() {
			So(levelVolume(nil), ShouldEqual, 0)
		})
	})
}
