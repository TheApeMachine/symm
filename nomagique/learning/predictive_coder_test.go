package learning

import (
	"math"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/data"
)

/*
coderFixture builds a coder over a small architecture with the requested
horizon depth, learning enabled.
*/
func coderFixture(horizon int) *PredictiveCoder {
	return NewPredictiveCoder(
		[]int{3, 6, 3}, horizon, NewDirectionalTarget(0), nil, 0, true, ReadoutAll,
	).(*PredictiveCoder)
}

/*
newDriver feeds a deterministic reference through one coder, advancing the
event clock monotonically so successive runs continue the same stream rather
than rewinding it.
*/
func newDriver(coder *PredictiveCoder) func(steps int) [12][]float64 {
	step, reference := 0.0, 100.0

	return func(steps int) [12][]float64 {
		var out [12][]float64

		for range steps {
			step++
			reference *= 1 + 0.01*math.Sin(step)
			hasReference := 0.0

			if step > 1 {
				hasReference = 1
			}

			out = data.Read[[12][]float64](coder.Next(data.NewValue([2][]float64{
				{reference, step, math.Sin(step)},
				{reference, hasReference, step, step},
			})))
			So(coder.Error(), ShouldBeNil)
		}

		return out
	}
}

func TestPredictiveCoderStep(t *testing.T) {
	Convey("Given a coder with a multi-step horizon", t, func() {
		const horizon = 8

		coder := coderFixture(horizon)

		Convey("every horizon row is trained, not only the next tick", func() {
			out := newDriver(coder)(40)

			// The whole point of a horizon: a next-tick call is not useful, so
			// each row must have learned from outcomes at its OWN distance.
			for step := 1; step <= horizon; step++ {
				So(step <= len(out[5]), ShouldBeTrue)
				So(out[5][step-1], ShouldEqual, 1)
			}
		})

		Convey("the supported horizon reaches the full declared depth", func() {
			out := newDriver(coder)(40)

			So(out[10][0], ShouldEqual, horizon)
			So(out[10][1], ShouldEqual, 1)
			So(len(out[11]), ShouldEqual, horizon)
		})

		Convey("the curve never runs past what the head has learned", func() {
			// Only a few steps in, most rows have seen no outcome yet. The
			// curve must stop at the learned reach.
			out := newDriver(coder)(4)

			So(out[10][0], ShouldBeLessThan, horizon)
			So(len(out[11]), ShouldEqual, int(out[10][0]))
			So(len(out[9]), ShouldBeLessThanOrEqualTo, int(out[10][0]))
		})

		Convey("an uncalibrated head publishes no curve at all", func() {
			out := newDriver(coderFixture(horizon))(1)

			So(out[10][0], ShouldEqual, 0)
			So(out[10][1], ShouldEqual, 0)
			So(out[11], ShouldBeEmpty)
		})

		Convey("a published curve is not mutated by the next step", func() {
			stream := newDriver(coder)
			stream(40)

			held := stream(1)[11]
			So(held, ShouldNotBeEmpty)

			retained := append([]float64(nil), held...)
			stream(1)

			So(held, ShouldResemble, retained)
		})

		Convey("one observation resolves once per horizon, not once in total", func() {
			out := newDriver(coder)(40)

			So(out[10][2], ShouldBeGreaterThan, 40)
		})

		Convey("a resolved observation reports the horizon it was scored at", func() {
			out := newDriver(coder)(40)
			summary := out[10]

			So(summary[6], ShouldEqual, 1)
			So(summary[7], ShouldBeGreaterThanOrEqualTo, 1)
			So(summary[7], ShouldBeLessThanOrEqualTo, horizon)
			So(summary[10], ShouldAlmostEqual, summary[9]-summary[8], 1e-12)
		})
	})

	Convey("Given a coder observing a stream with skipped steps", t, func() {
		coder := coderFixture(6)

		Convey("skipped caller steps still resolve issued predictions through the temporal ledger", func() {
			reference := 100.0
			var out [12][]float64

			for index, step := range []float64{1, 4, 7, 10, 13, 16, 19, 22} {
				reference *= 1.01
				hasReference := 0.0

				if index > 0 {
					hasReference = 1
				}

				out = data.Read[[12][]float64](coder.Next(data.NewValue([2][]float64{
					{reference, step, 1},
					{reference, hasReference, step, step},
				})))
				So(coder.Error(), ShouldBeNil)
			}

			So(out[10][2], ShouldBeGreaterThan, 0)
			So(coder.manifold.(*ResonanceManifold).taskScaleReady[0], ShouldBeTrue)
		})
	})

	Convey("Given a coder with no usable prior reference", t, func() {
		coder := coderFixture(4)

		Convey("nothing is issued or scored against an unanchored observation", func() {
			out := data.Read[[12][]float64](coder.Next(data.NewValue([2][]float64{
				{1, 2, 3},
				{100, 0, 1, 0},
			})))

			So(coder.Error(), ShouldBeNil)
			So(out[10][2], ShouldEqual, 0)
			So(out[10][6], ShouldEqual, 0)
			So(out[10][1], ShouldEqual, 0)
		})
	})

	Convey("Given a misconfigured coder", t, func() {
		Convey("an absent architecture is refused rather than panicking", func() {
			coder := NewPredictiveCoder(nil, 4, nil, nil, 0, false, ReadoutAll)

			for range coder.Next(data.NewValue([2][]float64{{1}, nil})) {
				t.Fatal("a coder without a manifold must yield nothing")
			}

			So(coder.Error(), ShouldNotBeNil)
		})

		Convey("an empty feature vector is refused", func() {
			coder := coderFixture(4)

			for range coder.Next(data.NewValue([2][]float64{nil, nil})) {
				t.Fatal("an empty feature vector must yield nothing")
			}

			So(coder.Error(), ShouldNotBeNil)
		})
	})
}

/*
TestPredictiveCoderRetainsBoundedPending proves the pending set is bounded by
the declared horizon, so a long-running symbol does not accumulate one live
forecast curve per tick forever.
*/
func TestPredictiveCoderRetainsBoundedPending(t *testing.T) {
	Convey("Given a coder driven far beyond its horizon depth", t, func() {
		const horizon = 5

		stream := newDriver(coderFixture(horizon))
		stream(200)

		Convey("it retains no more pending curves than the horizon allows", func() {
			So(stream(1)[10][3], ShouldBeLessThanOrEqualTo, horizon)
		})

		Convey("the pending set stops growing in steady state", func() {
			before := stream(1)[10][3]

			So(stream(100)[10][3], ShouldEqual, before)
		})
	})
}

/*
TestPredictiveCoderReadoutModes proves every readout mode is usable and that
the narrower ones really do shrink the head.

Each horizon holds a covariance matrix quadratic in the readout width, so at
high horizon depth this choice dominates the coder's memory.
*/
func TestPredictiveCoderReadoutModes(t *testing.T) {
	Convey("Given coders differing only in readout mode", t, func() {
		build := func(mode ReadoutMode) *PredictiveCoder {
			return NewPredictiveCoder(
				[]int{3, 6, 3}, 4, NewDirectionalTarget(0), nil, 0, true, mode,
			).(*PredictiveCoder)
		}

		Convey("every mode settles and forecasts without a dimension mismatch", func() {
			for _, mode := range []ReadoutMode{
				ReadoutAll, ReadoutLatents, ReadoutInnovations,
			} {
				out := newDriver(build(mode))(20)

				So(out[0], ShouldNotBeEmpty)
				So(len(out[1]), ShouldBeGreaterThan, 0)
			}
		})

		Convey("a narrower readout yields a narrower head", func() {
			wide := newDriver(build(ReadoutAll))(5)
			narrow := newDriver(build(ReadoutLatents))(5)

			So(narrow[0][8], ShouldBeLessThan, wide[0][8])
		})
	})
}

func BenchmarkPredictiveCoderStep(b *testing.B) {
	coder := coderFixture(300)
	step := 0.0
	input := [2][]float64{{100, 1, 0.5}, {100, 1, 0, 0}}

	for range 400 {
		step++
		input[1][2] = step

		for range coder.Next(data.NewValue(input)) {
		}
	}

	b.ResetTimer()
	b.ReportAllocs()

	for b.Loop() {
		step++
		input[1][0] = 100 + math.Mod(step, 7)
		input[1][2] = step

		for range coder.Next(data.NewValue(input)) {
		}
	}
}
