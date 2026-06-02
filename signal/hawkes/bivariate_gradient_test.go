package hawkes

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
)

func TestBivariateFitLogLikelihoodGradient(t *testing.T) {
	Convey("Given a fitted Hawkes process", t, func() {
		start := time.Now()
		stream := NewArrivalStream(
			[]time.Time{start, start.Add(time.Second), start.Add(2 * time.Second)},
			[]time.Time{start.Add(500 * time.Millisecond), start.Add(1500 * time.Millisecond)},
		)
		fit := BivariateFit{
			MuBuy: 1, MuSell: 1,
			AlphaBB: 0.1, AlphaBS: 0.05,
			AlphaSB: 0.05, AlphaSS: 0.1,
			Beta: 1,
		}

		logLikelihood, gradient, ok := fit.LogLikelihoodGradient(stream, start.Add(3*time.Second))

		Convey("It should return gradients for all parameters", func() {
			So(ok, ShouldBeTrue)
			So(logLikelihood, ShouldBeLessThan, 0)
			So(gradient[0], ShouldNotEqual, 0)
		})
	})
}
