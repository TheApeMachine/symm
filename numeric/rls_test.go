package numeric

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestNewRLSFilter(t *testing.T) {
	Convey("Given a valid dimension", t, func() {
		filter, err := NewRLSFilter(2, 1000)

		Convey("It should allocate the filter", func() {
			So(err, ShouldBeNil)
			So(filter, ShouldNotBeNil)
		})
	})

	Convey("Given a non-positive dimension", t, func() {
		_, err := NewRLSFilter(0, 1000)

		Convey("It should return an error", func() {
			So(err, ShouldNotBeNil)
		})
	})
}

func TestRLSFilterObserve(t *testing.T) {
	Convey("Given a simple linear relation", t, func() {
		filter, err := NewRLSFilter(1, 1000)
		So(err, ShouldBeNil)

		for step := 0; step < 32; step++ {
			feature := float64(step) / 32
			target := 2*feature + 1
			observeErr := filter.Observe([]float64{feature}, target)
			So(observeErr, ShouldBeNil)
		}

		forecast, predictErr := filter.Predict([]float64{0.5})

		Convey("It should learn the mapping", func() {
			So(predictErr, ShouldBeNil)
			So(forecast, ShouldAlmostEqual, 2, 0.25)
		})
	})
}
