package hawkes

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
)

func TestBivariateEstimatorFit(t *testing.T) {
	Convey("Given a trade arrival stream", t, func() {
		start := time.Now()
		stream := NewArrivalStream(
			[]time.Time{
				start,
				start.Add(time.Second),
				start.Add(2 * time.Second),
				start.Add(3 * time.Second),
			},
			[]time.Time{
				start.Add(500 * time.Millisecond),
				start.Add(1500 * time.Millisecond),
				start.Add(2500 * time.Millisecond),
				start.Add(3500 * time.Millisecond),
			},
		)
		estimator := NewBivariateEstimator(BivariateFit{})

		fit := estimator.Fit(stream, start.Add(4*time.Second))

		Convey("It should return a fit or empty when data is insufficient", func() {
			if fit.MuBuy > 0 {
				So(fit.valid(), ShouldBeTrue)
			}
		})
	})
}
