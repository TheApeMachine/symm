package temporal_test

import (
	"context"
	"math"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/temporal"
)

func TestExcursionWrite(t *testing.T) {
	ctx := context.Background()

	Convey("Given a path walked one step at a time", t, func() {
		client := temporal.Excursion_ServerToClient(temporal.NewExcursion(ctx))
		defer client.Release()

		walk := func(path []float64) error {
			for _, value := range path {
				err := client.Write(ctx, func(params temporal.Excursion_write_Params) error {
					params.SetValue(value)
					params.SetSigmas(3)
					params.SetHorizon(32)
					params.SetRetrace(0.5)
					params.SetFloor(0.003)
					return nil
				})

				if err != nil {
					return err
				}
			}

			return client.WaitStreaming()
		}

		read := func() (anchor, ignition, extremum, excursion float64, found bool, legs int64) {
			future, release := client.Done(ctx, nil)
			defer release()

			results, err := future.Struct()
			So(err, ShouldBeNil)

			return results.Anchor(), results.Ignition(), results.Extremum(),
				results.Excursion(), results.Found(), results.Legs()
		}

		// A trend is many small steps in one direction. The bar is read from
		// the size of a step, so a path that eases upward has a low bar to
		// clear and a violent one has a high bar.
		trend := func(from float64, steps int, rate float64) []float64 {
			path := make([]float64, 0, steps)
			value := from

			for range steps {
				value *= 1 + rate
				path = append(path, value)
			}

			return path
		}

		Convey("When the path rises and then gives back half of it", func() {
			path := trend(100, 60, 0.005)
			path = append(path, trend(path[len(path)-1], 40, -0.005)...)

			So(walk(path), ShouldBeNil)

			anchor, ignition, extremum, excursion, found, legs := read()

			Convey("Then it reports the move the path made", func() {
				So(found, ShouldBeTrue)
				So(legs, ShouldEqual, 1)
				So(excursion, ShouldBeGreaterThan, 0.30)
				So(extremum, ShouldBeGreaterThan, ignition)

				// The first leg has nothing before it, so its anchor is where
				// the path itself began.
				So(anchor, ShouldEqual, ignition)
			})
		})

		Convey("When the path is still running", func() {
			So(walk(trend(100, 60, 0.005)), ShouldBeNil)

			_, _, _, _, found, legs := read()

			// A beginning handed back as an outcome is the thing this must
			// never do.
			Convey("Then there is no move to report yet", func() {
				So(found, ShouldBeFalse)
				So(legs, ShouldEqual, 0)
			})
		})

		Convey("When the path only jitters", func() {
			path := make([]float64, 0, 64)

			for step := range 64 {
				path = append(path, 100+math.Mod(float64(step), 2)*0.01)
			}

			So(walk(path), ShouldBeNil)

			_, _, _, _, found, _ := read()

			Convey("Then nothing qualifies as a move", func() {
				So(found, ShouldBeFalse)
			})
		})

		Convey("When a second move follows the first", func() {
			path := trend(100, 60, 0.005)
			peak := path[len(path)-1]
			path = append(path, trend(peak, 60, -0.005)...)
			trough := path[len(path)-1]
			path = append(path, trend(trough, 40, 0.005)...)

			So(walk(path), ShouldBeNil)

			anchor, ignition, extremum, excursion, found, legs := read()

			// The leg running into ignition is what a precursor looks like,
			// so the anchor is where the previous leg began.
			Convey("Then the anchor is where the run into ignition started", func() {
				So(found, ShouldBeTrue)
				So(legs, ShouldBeGreaterThan, 1)
				So(anchor, ShouldBeLessThan, ignition)
				So(extremum, ShouldBeLessThan, ignition)
				So(excursion, ShouldBeLessThan, 0)
			})
		})

		Convey("When the path stands at zero", func() {
			err := client.Write(ctx, func(params temporal.Excursion_write_Params) error {
				params.SetValue(0)
				params.SetSigmas(3)
				params.SetHorizon(32)
				params.SetRetrace(0.5)
				return nil
			})
			So(err, ShouldBeNil)

			Convey("Then it refuses rather than reading a proportion of nothing", func() {
				So(client.WaitStreaming(), ShouldNotBeNil)
			})
		})
	})
}
