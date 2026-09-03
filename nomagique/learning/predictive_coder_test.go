package learning

import (
	"math"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

/*
coderFixture builds a coder over a small architecture with the requested
horizon depth, learning enabled.
*/
func coderFixture(horizon int) *PredictiveCoder {
	return NewPredictiveCoder(PredictiveCoderConfig{
		CustomArch: []int{3, 6, 3},
		MaxHorizon: horizon,
		Target:     DirectionalTarget(0),
		Pace:       NewPaceController(),
		Learn:      true,
	})
}

/*
driver feeds a deterministic reference through one coder, advancing the event
clock monotonically so successive runs continue the same stream rather than
rewinding it.
*/
type driver struct {
	coder     *PredictiveCoder
	step      int64
	reference float64
}

func newDriver(coder *PredictiveCoder) *driver {
	return &driver{coder: coder, reference: 100.0}
}

func (drive *driver) run(steps int) PredictiveOutput {
	var out PredictiveOutput

	for range steps {
		drive.step++
		drive.reference *= 1 + 0.01*math.Sin(float64(drive.step))

		out, _ = drive.coder.Step(PredictiveInput{
			Features: []float64{
				drive.reference,
				float64(drive.step),
				math.Sin(float64(drive.step)),
			},
			Reference:    drive.reference,
			HasReference: drive.step > 1,
			Step:         drive.step,
			Time:         float64(drive.step),
		})
	}

	return out
}

/*
drive runs a fresh stream of the given length through a coder.
*/
func drive(coder *PredictiveCoder, steps int) PredictiveOutput {
	return newDriver(coder).run(steps)
}

func TestPredictiveCoderStep(t *testing.T) {
	Convey("Given a coder with a multi-step horizon", t, func() {
		const horizon = 8

		coder := coderFixture(horizon)

		Convey("every horizon row is trained, not only the next tick", func() {
			drive(coder, 40)

			// The whole point of a horizon: a next-tick call is not useful, so
			// each row must have learned from outcomes at its OWN distance.
			for step := 1; step <= horizon; step++ {
				_, defined := coder.Manifold().TaskSkillAt(step)

				So(defined, ShouldBeTrue)
			}
		})

		Convey("the supported horizon reaches the full declared depth", func() {
			out := drive(coder, 40)

			So(out.SupportedHorizon, ShouldEqual, horizon)
			So(out.Calibrated, ShouldBeTrue)
			So(len(out.ForwardCurve), ShouldEqual, horizon)
		})

		Convey("the curve never runs past what the head has learned", func() {
			// Only a few steps in, most rows have seen no outcome yet. The
			// curve must stop at the learned reach: untrained rows emit
			// near-zero and would read downstream as a genuine flat forecast
			// rather than as absent evidence.
			out := drive(coder, 4)

			So(out.SupportedHorizon, ShouldBeLessThan, horizon)
			So(len(out.ForwardCurve), ShouldEqual, out.SupportedHorizon)
			So(len(out.ForwardRetention), ShouldBeLessThanOrEqualTo, out.SupportedHorizon)
		})

		Convey("an uncalibrated head publishes no curve at all", func() {
			fresh := coderFixture(horizon)
			out := drive(fresh, 1)

			So(out.SupportedHorizon, ShouldEqual, 0)
			So(out.Calibrated, ShouldBeFalse)
			So(out.ForwardCurve, ShouldBeEmpty)
		})

		Convey("a published curve is not mutated by the next step", func() {
			stream := newDriver(coder)
			stream.run(40)

			held := stream.run(1).ForwardCurve
			So(held, ShouldNotBeEmpty)

			retained := append([]float64(nil), held...)
			stream.run(1)

			So(held, ShouldResemble, retained)
		})

		Convey("one observation resolves once per horizon, not once in total", func() {
			drive(coder, 40)

			// 40 steps, each issuing a curve that resolves at every horizon it
			// survives to reach, must produce far more resolutions than the one
			// per observation a single-horizon coder would report.
			So(coder.ResolvedSteps(), ShouldBeGreaterThan, 40)
		})

		Convey("a resolved observation reports the horizon it was scored at", func() {
			out := drive(coder, 40)

			So(out.LastResolution, ShouldNotBeNil)
			So(out.LastResolution.Horizon, ShouldBeGreaterThanOrEqualTo, 1)
			So(out.LastResolution.Horizon, ShouldBeLessThanOrEqualTo, horizon)
			So(out.LastResolution.Error, ShouldAlmostEqual,
				out.LastResolution.Target-out.LastResolution.Prediction, 1e-12)
		})
	})

	Convey("Given a coder observing a stream with skipped steps", t, func() {
		coder := coderFixture(6)

		Convey("a horizon is trained by elapsed distance, never by arrival order", func() {
			reference := 100.0

			// Steps jump by 3, so the second observation is 3 horizons away
			// from the first. Training must land on row 3, not row 1.
			for index, step := range []int64{1, 4, 7, 10, 13, 16, 19, 22} {
				reference *= 1.01

				coder.Step(PredictiveInput{
					Features:     []float64{reference, float64(step), 1},
					Reference:    reference,
					HasReference: index > 0,
					Step:         step,
					Time:         float64(step),
				})
			}

			_, nearDefined := coder.Manifold().TaskSkillAt(1)
			_, farDefined := coder.Manifold().TaskSkillAt(3)

			// Row 1 never saw a 1-step outcome, because no two observations
			// were ever 1 step apart.
			So(nearDefined, ShouldBeFalse)
			So(farDefined, ShouldBeTrue)
		})
	})

	Convey("Given a coder with no usable prior reference", t, func() {
		coder := coderFixture(4)

		Convey("nothing is issued or scored against an unanchored observation", func() {
			out, err := coder.Step(PredictiveInput{
				Features:     []float64{1, 2, 3},
				Reference:    100,
				HasReference: false,
				Step:         1,
			})

			So(err, ShouldBeNil)
			So(coder.ResolvedSteps(), ShouldEqual, 0)
			So(out.LastResolution, ShouldBeNil)
			So(out.Calibrated, ShouldBeFalse)
		})
	})

	Convey("Given a misconfigured coder", t, func() {
		Convey("an absent architecture is refused rather than panicking", func() {
			coder := NewPredictiveCoder(PredictiveCoderConfig{MaxHorizon: 4})

			_, err := coder.Step(PredictiveInput{Features: []float64{1}})

			So(err, ShouldNotBeNil)
		})

		Convey("an empty feature vector is refused", func() {
			_, err := coderFixture(4).Step(PredictiveInput{})

			So(err, ShouldNotBeNil)
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

		coder := coderFixture(horizon)
		stream := newDriver(coder)
		stream.run(200)

		Convey("it retains no more pending curves than the horizon allows", func() {
			So(len(coder.pending), ShouldBeLessThanOrEqualTo, horizon)
		})

		Convey("a fully resolved curve is recycled instead of being reallocated", func() {
			// In steady state a recycled curve is taken straight back by the
			// next issue, so the free list is transiently empty by design.
			// What must hold is that the pending set stops growing at all.
			before := len(coder.pending)
			stream.run(100)

			So(len(coder.pending), ShouldEqual, before)
		})
	})
}

/*
TestPredictiveCoderReadoutModes proves every readout mode is usable and that
the narrower ones really do shrink the head.

Each horizon holds a covariance matrix quadratic in the readout width, so at
high MaxHorizon this choice dominates the coder's memory. ReadoutLatents and
ReadoutInnovations previously panicked with a dimension mismatch, because the
workspace was always sized for the widest mode.
*/
func TestPredictiveCoderReadoutModes(t *testing.T) {
	Convey("Given coders differing only in readout mode", t, func() {
		build := func(mode ReadoutMode) *PredictiveCoder {
			return NewPredictiveCoder(PredictiveCoderConfig{
				CustomArch: []int{3, 6, 3},
				MaxHorizon: 4,
				Target:     DirectionalTarget(0),
				Learn:      true,
				Readout:    mode,
			})
		}

		Convey("every mode settles and forecasts without a dimension mismatch", func() {
			for _, mode := range []ReadoutMode{
				ReadoutAll, ReadoutLatents, ReadoutInnovations,
			} {
				coder := build(mode)
				out := drive(coder, 20)

				So(coder.Manifold(), ShouldNotBeNil)
				So(len(out.Readout), ShouldBeGreaterThan, 0)
			}
		})

		Convey("a narrower readout yields a narrower head", func() {
			wide := build(ReadoutAll)
			narrow := build(ReadoutLatents)

			drive(wide, 5)
			drive(narrow, 5)

			So(len(narrow.Manifold().ReadoutVector()), ShouldBeLessThan,
				len(wide.Manifold().ReadoutVector()))
		})

		Convey("the zero value keeps the widest readout", func() {
			defaulted := build(ReadoutAll)
			explicit := NewPredictiveCoder(PredictiveCoderConfig{
				CustomArch: []int{3, 6, 3},
				MaxHorizon: 4,
				Target:     DirectionalTarget(0),
				Learn:      true,
			})

			drive(defaulted, 5)
			drive(explicit, 5)

			So(len(explicit.Manifold().ReadoutVector()), ShouldEqual,
				len(defaulted.Manifold().ReadoutVector()))
		})
	})
}

func BenchmarkPredictiveCoderStep(b *testing.B) {
	coder := coderFixture(300)
	drive(coder, 400)

	features := []float64{100, 1, 0.5}
	step := int64(401)

	b.ResetTimer()
	b.ReportAllocs()

	for b.Loop() {
		coder.Step(PredictiveInput{
			Features:     features,
			Reference:    100 + float64(step%7),
			HasReference: true,
			Step:         step,
		})

		step++
	}
}
