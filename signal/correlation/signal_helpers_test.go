package correlation

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestPeerHerdingPercentile(t *testing.T) {
	Convey("Given peer correlation dispersion", t, func() {
		tightPeers := []float64{0.81, 0.82, 0.80, 0.81}
		widePeers := []float64{0.10, 0.80, -0.20, 0.55}

		Convey("It should raise the herd gate with wider peer dispersion", func() {
			tightPercentile := peerHerdingPercentile(tightPeers)
			widePercentile := peerHerdingPercentile(widePeers)

			So(widePercentile, ShouldBeGreaterThan, tightPercentile)
			So(tightPercentile, ShouldBeGreaterThan, 0)
			So(widePercentile, ShouldBeLessThanOrEqualTo, 1)
		})
	})
}
