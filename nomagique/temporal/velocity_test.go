package temporal_test

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/temporal"
)

func TestVelocityNext(t *testing.T) {
	Convey("Velocity measures successive native positions over seconds", t, func() {
		op := temporal.NewVelocity()
		mapping := data.NewMap(
			"position", "position",
			"time", "time",
			"velocity", "velocity",
		)

		fixtures := []struct {
			position float64
			at       float64
			velocity float64
		}{
			{position: 10, at: 1, velocity: 0},
			{position: 13, at: 2, velocity: 3},
			{position: 16, at: 2, velocity: 0},
			{position: 8, at: 3, velocity: -8},
		}

		for _, fixture := range fixtures {
			values := data.NewOutputMap()
			values.Values["position"] = fixture.position
			values.Values["time"] = fixture.at
			adapter := data.NewAdapter(nil, data.NewState(mapping, values))

			for range op.Next(data.NewValue(adapter)) {
			}

			So(values.Values["velocity"], ShouldEqual, fixture.velocity)
		}
	})
}
