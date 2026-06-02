package hawkes

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
)

func TestMaximizeLikelihood(t *testing.T) {
	Convey("Given fit context and seed", t, func() {
		start := time.Now()
		stream := NewArrivalStream(
			[]time.Time{start, start.Add(time.Second), start.Add(2 * time.Second), start.Add(3 * time.Second)},
			[]time.Time{start.Add(500 * time.Millisecond), start.Add(1500 * time.Millisecond), start.Add(2500 * time.Millisecond)},
		)
		context, ok := NewFitContext(stream, start.Add(4*time.Second))

		So(ok, ShouldBeTrue)

		estimator := NewBivariateEstimator(BivariateFit{})
		seeds := estimator.multiStartSeeds(context)

		So(len(seeds), ShouldBeGreaterThan, 0)

		candidate := estimator.maximizeLikelihood(stream, start.Add(4*time.Second), context, seeds[0])

		Convey("It should optimize toward positive rates", func() {
			So(candidate.MuBuy, ShouldBeGreaterThan, 0)
			So(candidate.MuSell, ShouldBeGreaterThan, 0)
		})
	})
}
