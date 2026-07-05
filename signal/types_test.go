package signal

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/logic"
)

func TestMeasurementMerge(t *testing.T) {
	Convey("Given a signal measurement", t, func() {
		measurement := NewMeasurement(logic.SourceFluid, "BTC/USD", time.Now().UTC())

		Convey("When merging a slice longer than ten values", func() {
			measurement.Merge(map[string]any{
				"probabilities": []float64{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10},
			})

			Convey("Then indexes should be decimal keys and string storage should be initialized", func() {
				So(measurement.Output["probabilities_10"], ShouldEqual, 10)
				So(measurement.Strings, ShouldNotBeNil)
			})
		})
	})
}
