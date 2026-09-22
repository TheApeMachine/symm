package hawkes_test

import (
	"context"
	"math"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/statistic/hawkes"
)

/*
perturb returns a copy of values with one entry displaced, so a caller can
difference the likelihood along a single parameter direction.
*/
func perturb(values []float64, index int, delta float64) []float64 {
	moved := append([]float64(nil), values...)
	moved[index] += delta
	return moved
}

func TestLogLikelihoodServer_Write(t *testing.T) {
	Convey("Given a simulated two-component Hawkes path", t, func() {
		baseline := []float64{0.6, 0.4}
		excitation := []float64{0.9, 0.3, 0.2, 0.8}
		decay := 1.7
		path := simulate(2, baseline, excitation, decay, 400, 20260922)

		So(len(path.times), ShouldBeGreaterThan, 200)

		Convey("When the likelihood is evaluated at the generating parameters", func() {
			value, gradient, defined := likelihood(t, path, baseline, excitation, decay)

			Convey("Then it is defined and finite", func() {
				So(defined, ShouldBeTrue)
				So(math.IsNaN(value), ShouldBeFalse)
				So(math.IsInf(value, 0), ShouldBeFalse)
			})

			Convey("Then the gradient carries one entry per parameter", func() {
				So(len(gradient), ShouldEqual, 2+4+1)
			})

			Convey("Then every analytic partial matches a central difference", func() {
				// The step is large enough that the difference of two
				// likelihoods is not lost to cancellation, and small enough
				// that the second-order term stays under the tolerance.
				step := 1e-5

				for index := range baseline {
					up, _, _ := likelihood(t, path, perturb(baseline, index, step), excitation, decay)
					down, _, _ := likelihood(t, path, perturb(baseline, index, -step), excitation, decay)
					numeric := (up - down) / (2 * step)

					So(math.Abs(numeric-gradient[index]), ShouldBeLessThan, 1e-4*math.Max(1, math.Abs(numeric)))
				}

				for index := range excitation {
					up, _, _ := likelihood(t, path, baseline, perturb(excitation, index, step), decay)
					down, _, _ := likelihood(t, path, baseline, perturb(excitation, index, -step), decay)
					numeric := (up - down) / (2 * step)

					So(math.Abs(numeric-gradient[2+index]), ShouldBeLessThan, 1e-4*math.Max(1, math.Abs(numeric)))
				}

				up, _, _ := likelihood(t, path, baseline, excitation, decay+step)
				down, _, _ := likelihood(t, path, baseline, excitation, decay-step)
				numeric := (up - down) / (2 * step)

				So(math.Abs(numeric-gradient[len(gradient)-1]), ShouldBeLessThan, 1e-4*math.Max(1, math.Abs(numeric)))
			})
		})

		Convey("When the generating parameters are compared against wrong ones", func() {
			truth, _, _ := likelihood(t, path, baseline, excitation, decay)
			flat, _, _ := likelihood(t, path, []float64{1.4, 1.4}, []float64{0, 0, 0, 0}, decay)

			Convey("Then the process that produced the path explains it better", func() {
				So(truth, ShouldBeGreaterThan, flat)
			})
		})

		Convey("When excitation is strong enough to drive an intensity negative", func() {
			_, _, defined := likelihood(t, path, []float64{0.6, 0.4}, []float64{-9, -9, -9, -9}, decay)

			Convey("Then no likelihood is reported rather than a substituted one", func() {
				So(defined, ShouldBeFalse)
			})
		})

		Convey("When a refused parameter set follows an accepted one", func() {
			client := hawkes.LogLikelihood_ServerToClient(hawkes.NewLogLikelihood())
			ctx := context.Background()
			integral, integralDecay := integrate(t, path, decay)

			evaluate := func(baselines []float64) (float64, bool) {
				err := client.Write(ctx, func(params hawkes.LogLikelihood_write_Params) error {
					if err := writeFloats(params.NewTimes, path.times); err != nil {
						return err
					}

					if err := writeFloats(params.NewComponents, path.components); err != nil {
						return err
					}

					if err := writeFloats(params.NewBaseline, baselines); err != nil {
						return err
					}

					if err := writeFloats(params.NewExcitation, excitation); err != nil {
						return err
					}

					if err := writeFloats(params.NewIntegral, integral); err != nil {
						return err
					}

					if err := writeFloats(params.NewIntegralDecayDerivative, integralDecay); err != nil {
						return err
					}

					params.SetOrigin(path.origin)
					params.SetHorizon(path.horizon)
					params.SetDecay(decay)
					params.SetDimension(2)
					return nil
				})
				So(err, ShouldBeNil)
				So(client.WaitStreaming(), ShouldBeNil)

				future, release := client.Done(ctx, nil)
				defer release()
				results, err := future.Struct()
				So(err, ShouldBeNil)
				return results.Value(), results.Defined()
			}

			accepted, acceptedOK := evaluate(baseline)
			refusedValue, refusedOK := evaluate([]float64{-5, -5})

			Convey("Then the stale value does not survive as the new answer", func() {
				So(acceptedOK, ShouldBeTrue)
				So(refusedOK, ShouldBeFalse)
				So(refusedValue, ShouldNotEqual, accepted)
			})
		})

		Convey("When the decay rate is not positive", func() {
			_, _, defined := likelihood(t, path, baseline, excitation, 0)

			Convey("Then the kernel has no scale and no likelihood is reported", func() {
				So(defined, ShouldBeFalse)
			})
		})

		Convey("When a component label falls outside the process", func() {
			strayed := path
			strayed.components = append([]float64(nil), path.components...)
			strayed.components[len(strayed.components)/2] = 7
			_, _, defined := likelihood(t, strayed, baseline, excitation, decay)

			Convey("Then the arrival is not silently attributed elsewhere", func() {
				So(defined, ShouldBeFalse)
			})
		})
	})
}
