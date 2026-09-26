package temporal_test

import (
	"context"
	"math"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/temporal"
	"github.com/theapemachine/symm/tests/market"
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

			if results.Which() == temporal.ExcursionResult_Which_none {
				return 0, 0, 0, 0, false, results.Legs()
			}

			move := results.Move()
			return move.Anchor(), move.Ignition(), move.Extremum(), move.Excursion(), true, results.Legs()
		}

		// A trend is many small steps in one direction. The bar is read from
		// the size of a step, so a path that eases upward has a low bar to
		// clear and a violent one has a high bar.

		Convey("When the path rises and then gives back half of it", func() {
			path := market.Trend(100, 60, 0.005)
			path = append(path, market.Trend(path[len(path)-1], 40, -0.005)...)

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
			So(walk(market.Trend(100, 60, 0.005)), ShouldBeNil)

			_, _, _, _, found, legs := read()

			// A beginning handed back as an outcome is the thing this must
			// never do.
			Convey("Then there is no move to report yet", func() {
				So(found, ShouldBeFalse)
				So(legs, ShouldEqual, 0)
			})
		})

		Convey("When the first leg falls without reversing", func() {
			So(walk(market.Trend(100, 60, -0.005)), ShouldBeNil)
			_, _, _, _, found, legs := read()
			So(found, ShouldBeFalse)
			So(legs, ShouldEqual, 0)
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
			path := market.Trend(100, 60, 0.005)
			peak := path[len(path)-1]
			path = append(path, market.Trend(peak, 60, -0.005)...)
			trough := path[len(path)-1]
			path = append(path, market.Trend(trough, 40, 0.005)...)

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
				return nil
			})
			So(err, ShouldBeNil)

			Convey("Then it refuses rather than reading a proportion of nothing", func() {
				So(client.WaitStreaming(), ShouldNotBeNil)
			})
		})
	})
}

func TestExcursionDone(t *testing.T) {
	Convey("Given measurements from a flat path followed by two opposing runs", t, func() {
		ctx := context.Background()
		client := temporal.Excursion_ServerToClient(temporal.NewExcursion(ctx))
		defer client.Release()
		path := []float64{100, 100, 100, 101, 102, 104, 108, 104, 100, 96, 92, 98, 104}

		for _, value := range path {
			So(client.Write(ctx, func(params temporal.Excursion_write_Params) error {
				params.SetValue(value)
				return nil
			}), ShouldBeNil)
			So(client.WaitStreaming(), ShouldBeNil)
		}

		future, release := client.Done(ctx, nil)
		defer release()
		result, err := future.Struct()
		So(err, ShouldBeNil)
		So(result.Steps(), ShouldEqual, len(path)-1)
		So(result.Floor(), ShouldAlmostEqual, math.Log(102.0/101.0))
		So(result.Horizon(), ShouldBeGreaterThan, 0)
		So(result.Sigma(), ShouldBeGreaterThan, 0)
		So(result.Legs(), ShouldBeGreaterThan, 0)
		So(result.Which(), ShouldEqual, temporal.ExcursionResult_Which_move)

		Convey("Then reading again cannot report the same completed move", func() {
			future, release := client.Done(ctx, nil)
			defer release()
			result, err := future.Struct()
			So(err, ShouldBeNil)
			So(result.Which(), ShouldEqual, temporal.ExcursionResult_Which_none)
			So(result.Steps(), ShouldEqual, len(path)-1)
		})
	})
}

func BenchmarkExcursionWrite(b *testing.B) {
	ctx := context.Background()
	client := temporal.Excursion_ServerToClient(temporal.NewExcursion(ctx))
	defer client.Release()
	// A complete rise, decline and recovery, repeated through the public protocol.
	path := []float64{100, 101, 103, 107, 109, 108, 104, 98, 94, 96, 99, 100}
	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		for _, value := range path {
			err := client.Write(ctx, func(params temporal.Excursion_write_Params) error {
				params.SetValue(value)
				return nil
			})

			if err != nil {
				b.Fatal(err)
			}

			if err := client.WaitStreaming(); err != nil {
				b.Fatal(err)
			}
			future, release := client.Done(ctx, nil)
			_, err = future.Struct()
			release()

			if err != nil {
				b.Fatal(err)
			}
		}
	}
}

func TestExcursionWriteStamped(t *testing.T) {
	Convey("Excursion boundaries retain global sequence positions across interleaved feeds", t, func() {
		client := temporal.Excursion_ServerToClient(temporal.NewExcursion(context.Background()))
		defer client.Release()
		path := market.Reversal()
		observed := make(map[int64]float64, len(path))
		moves := 0
		for index, value := range path {
			// Two observations from other feeds occur between these ticker observations.
			sequence := int64(100 + index*3)
			observed[sequence] = value
			So(client.Write(context.Background(), func(args temporal.Excursion_write_Params) error {
				args.SetValue(value)
				args.SetEpoch(91)
				args.SetSequence(sequence)
				return args.SetScope("BTC/USD")
			}), ShouldBeNil)
			So(client.WaitStreaming(), ShouldBeNil)
			future, release := client.Done(context.Background(), nil)
			result, err := future.Struct()
			So(err, ShouldBeNil)
			if result.Which() == temporal.ExcursionResult_Which_move {
				move := result.Move()
				moves++
				So(move.ConfirmationSequence(), ShouldEqual, sequence)
				So(observed[move.AnchorSequence()], ShouldEqual, move.Anchor())
				So(observed[move.IgnitionSequence()], ShouldEqual, move.Ignition())
				So(observed[move.ExtremumSequence()], ShouldEqual, move.Extremum())
				So(move.ExtremumSequence(), ShouldBeLessThan, move.ConfirmationSequence())
				So(result.Epoch(), ShouldEqual, 91)
			}
			release()
		}
		So(moves, ShouldBeGreaterThanOrEqualTo, 2)
	})
}
