package temporal

import (
	"context"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestVelocityWrite(t *testing.T) {
	Convey("Given a series moving over time", t, func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		observe := func(points [][2]float64) Velocity_done_Results {
			client := Velocity_ServerToClient(NewVelocity())
			t.Cleanup(client.Release)

			for _, point := range points {
				err := client.Write(ctx, func(params Velocity_write_Params) error {
					params.SetTs(point[0])
					params.SetVal(point[1])
					return nil
				})
				So(err, ShouldBeNil)
			}

			future, release := client.Done(ctx, nil)
			t.Cleanup(release)

			results, err := future.Struct()
			So(err, ShouldBeNil)

			return results
		}

		Convey("When it rises at a steady rate", func() {
			results := observe([][2]float64{{0, 0}, {1, 2}, {2, 4}, {3, 6}})

			Convey("Then the rate is the slope across every observation", func() {
				So(results.Out(), ShouldAlmostEqual, 2, 1e-9)
				So(results.Defined(), ShouldBeTrue)
			})

			Convey("Then a move with no scatter is overwhelmingly confident", func() {
				So(results.Snr(), ShouldBeGreaterThan, 1e6)
			})
		})

		Convey("When the same rise is buried in scatter", func() {
			steady := observe([][2]float64{{0, 0}, {1, 1}, {2, 2}, {3, 3}, {4, 4}})
			// The same rise, with scatter placed so it cancels against time and
			// leaves the slope exactly where the steady series has it.
			noisy := observe([][2]float64{{0, 0}, {1, 7}, {2, 2}, {3, 9}, {4, 4}})

			Convey("Then both report a rate", func() {
				So(steady.Out(), ShouldAlmostEqual, 1, 1e-9)
				So(noisy.Out(), ShouldAlmostEqual, 1, 1e-9)
			})

			Convey("Then only the steady one is evidence of a move", func() {
				// The same slope from two very different series. A rate alone
				// cannot tell them apart, which is why the confidence travels
				// with it.
				So(noisy.Snr(), ShouldBeLessThan, 1)
				So(steady.Snr(), ShouldBeGreaterThan, noisy.Snr())
			})
		})

		Convey("When only one observation has arrived", func() {
			results := observe([][2]float64{{0, 5}})

			Convey("Then no rate is claimed", func() {
				So(results.Defined(), ShouldBeFalse)
			})
		})

		Convey("When every observation shares one instant", func() {
			results := observe([][2]float64{{7, 1}, {7, 2}, {7, 3}})

			Convey("Then no rate is claimed, rather than an infinite one", func() {
				So(results.Defined(), ShouldBeFalse)
			})
		})
	})
}
