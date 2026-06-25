package manifold

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
)

func TestResolveFeaturesCache(testingTB *testing.T) {
	Convey("Given a signal with cached features", testingTB, func() {
		signal := &Signal{
			featureCache: featureCacheEntry{
				scope:      "BTC/USD",
				eventStamp: time.Unix(0, 123).UnixNano(),
				pressure:   1.5,
				coherence:  2.5,
				guidance:   3.5,
				viscosity:  4.5,
				ok:         true,
			},
		}

		pressure, coherence, guidance, viscosity, ok := signal.resolveFeatures(
			"BTC/USD",
			time.Unix(0, 123),
		)

		Convey("It should return the cached reading without tree access", func() {
			So(ok, ShouldBeTrue)
			So(pressure, ShouldEqual, 1.5)
			So(coherence, ShouldEqual, 2.5)
			So(guidance, ShouldEqual, 3.5)
			So(viscosity, ShouldEqual, 4.5)
		})
	})
}
