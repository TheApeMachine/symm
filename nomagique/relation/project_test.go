package relation

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/data"
)

func TestProject(t *testing.T) {
	Convey("Given a finalized measurement and the coordinates it projects", t, func() {
		measurement := data.NewMeasurement(1, "TEST/USD", "cvd", 1, 1)
		measurement.At = time.Unix(10, 0).UTC()
		measurement.From = time.Unix(9, 0).UTC()
		measurement.Write(
			data.NewMetric("signed_net_fraction:buy", 0.25, data.UnitRatio, data.TimescaleTick),
		)

		store := NewObservationStore(8)
		project := NewProject(store, 7, [4]string{
			"signed_net_fraction", "buy", string(data.UnitRatio), string(data.TimescaleTick),
		})

		var yielded int

		for range project.Next(data.NewValue(measurement)) {
			yielded++
		}

		Convey("It stamps identity from the measurement and reads the metric by key", func() {
			So(project.Error(), ShouldBeNil)
			So(yielded, ShouldEqual, 1)

			resident := residentCopy(store)
			So(resident, ShouldHaveLength, 1)
			So(resident["TEST/USD|cvd|signed_net_fraction|buy||ratio|tick|7"], ShouldResemble, []float64{
				float64(time.Unix(10, 0).UnixNano()), 0.25,
			})
		})

		Convey("A metric the measurement does not carry rejects it as a whole", func() {
			missing := NewProject(store, 7, [4]string{"absent", "", "", ""})

			for range missing.Next(data.NewValue(measurement)) {
			}

			So(missing.Error(), ShouldNotBeNil)
			So(residentCopy(store), ShouldHaveLength, 1)
		})
	})
}
