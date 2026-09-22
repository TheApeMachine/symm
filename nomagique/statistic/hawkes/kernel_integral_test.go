package hawkes_test

import (
	"math"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestKernelIntegralServer_Write(t *testing.T) {
	Convey("Given arrivals inside and before an observation window", t, func() {
		path := realisation{
			times:      []float64{0, 1, 2, 3},
			components: []float64{0, 1, 0, 1},
			dimension:  2,
			origin:     1,
			horizon:    4,
		}
		decay := 0.8

		Convey("When the kernel is integrated over the window", func() {
			support, derivative := integrate(t, path, decay)

			Convey("Then it matches the same integral taken numerically", func() {
				// The closed form is only worth having if it agrees with
				// summing the kernel step by step across the window.
				steps := 400000
				width := (path.horizon - path.origin) / float64(steps)
				numeric := make([]float64, 2)

				for step := 0; step < steps; step++ {
					at := path.origin + (float64(step)+0.5)*width

					for index, eventTime := range path.times {
						if eventTime > at {
							continue
						}

						numeric[int(path.components[index])] += math.Exp(-decay*(at-eventTime)) * width * decay
					}
				}

				for index := range support {
					So(math.Abs(support[index]-numeric[index]), ShouldBeLessThan, 1e-4)
				}
			})

			Convey("Then the decay derivative matches a central difference", func() {
				step := 1e-6
				up, _ := integrate(t, path, decay+step)
				down, _ := integrate(t, path, decay-step)

				for index := range derivative {
					numeric := (up[index] - down[index]) / (2 * step)
					So(math.Abs(numeric-derivative[index]), ShouldBeLessThan, 1e-5)
				}
			})

			Convey("Then an arrival before the origin contributes only its in-window decay", func() {
				// The arrival at 0 precedes the window, so only the part of
				// its kernel falling inside (1, 4] counts. Reading the whole
				// of it would overstate the compensator by the decay it
				// already spent before the window opened.
				prehistory := math.Exp(-decay*1) - math.Exp(-decay*4)
				inside := 1 - math.Exp(-decay*2)
				So(math.Abs(support[0]-(prehistory+inside)), ShouldBeLessThan, 1e-12)
				So(prehistory, ShouldBeLessThan, 1-math.Exp(-decay*4))
			})
		})

		Convey("When the decay rate is not positive", func() {
			support, derivative := integrate(t, path, 0)

			Convey("Then the kernel has no scale and nothing is reported", func() {
				So(support, ShouldBeEmpty)
				So(derivative, ShouldBeEmpty)
			})
		})
	})
}
