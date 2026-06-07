package reasoning

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/optimizer/replay"
)

func TestWalletVelocityScore(t *testing.T) {
	Convey("Given equal EUR, faster turnover should score higher", t, func() {
		fast := replay.ReplayResult{
			RealizedEUR:   10,
			TotalTicks:    100,
			ExposureTicks: 10,
		}
		slow := replay.ReplayResult{
			RealizedEUR:   10,
			TotalTicks:    100,
			ExposureTicks: 80,
		}

		Convey("walletVelocityScore should prefer the fast forest", func() {
			So(walletVelocityScore(fast), ShouldBeGreaterThan, walletVelocityScore(slow))
		})
	})
}
