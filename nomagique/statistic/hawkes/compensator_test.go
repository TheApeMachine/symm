package hawkes_test

import (
	"context"
	"math"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/statistic/hawkes"
)

/*
compensatorOf drives the Compensator node over one parameter set and
integrated kernel support.
*/
func compensatorOf(
	t *testing.T,
	baseline, excitation, support []float64,
	span, decay float64,
	dimension int,
) []float64 {
	t.Helper()
	ctx := context.Background()
	client := hawkes.Compensator_ServerToClient(hawkes.NewCompensator())

	err := client.Write(ctx, func(params hawkes.Compensator_write_Params) error {
		if err := writeFloats(params.NewBaseline, baseline); err != nil {
			return err
		}

		if err := writeFloats(params.NewExcitation, excitation); err != nil {
			return err
		}

		if err := writeFloats(params.NewSupport, support); err != nil {
			return err
		}

		params.SetSpan(span)
		params.SetDecay(decay)
		params.SetDimension(int32(dimension))
		return nil
	})

	if err != nil {
		t.Fatalf("compensator write: %v", err)
	}

	if err := client.WaitStreaming(); err != nil {
		t.Fatalf("compensator stream: %v", err)
	}

	future, release := client.Done(ctx, nil)
	defer release()
	results, err := future.Struct()

	if err != nil {
		t.Fatalf("compensator done: %v", err)
	}

	list, err := results.Compensator()

	if err != nil {
		t.Fatalf("compensator list: %v", err)
	}

	return readList(list.Len(), list.At)
}

func TestCompensatorServer_Write(t *testing.T) {
	Convey("Given a window and a parameter set", t, func() {
		path := realisation{
			times:      []float64{0, 0.7, 1.4, 2.9, 3.3},
			components: []float64{0, 1, 0, 1, 0},
			dimension:  2,
			origin:     0,
			horizon:    5,
		}
		baseline := []float64{0.4, 0.2}
		excitation := []float64{0.5, 0.3, 0.2, 0.6}
		decay := 1.1
		span := path.horizon - path.origin

		Convey("When the compensator is assembled from the integrated kernel", func() {
			integral, _ := integrate(t, path, decay)
			compensator := compensatorOf(t, baseline, excitation, integral, span, decay, 2)

			Convey("Then it equals the intensity integrated across the window", func() {
				// The compensator is only meaningful if it is the area under
				// the very intensity the Intensity node reports. Walking the
				// window and summing that intensity is the definition it has
				// to meet.
				steps := 200000
				width := span / float64(steps)
				numeric := make([]float64, 2)

				for step := 0; step < steps; step++ {
					at := path.origin + (float64(step)+0.5)*width
					support := excitationOf(t, path, at, decay)
					intensity := intensityOf(t, baseline, excitation, support, 2)

					for index := range numeric {
						numeric[index] += intensity[index] * width
					}
				}

				for index := range compensator {
					So(math.Abs(compensator[index]-numeric[index]), ShouldBeLessThan, 1e-3)
				}
			})
		})

		Convey("When the process has no excitation", func() {
			integral, _ := integrate(t, path, decay)
			compensator := compensatorOf(t, baseline, []float64{0, 0, 0, 0}, integral, span, decay, 2)

			Convey("Then it is the baseline carried across the window", func() {
				So(math.Abs(compensator[0]-baseline[0]*span), ShouldBeLessThan, 1e-12)
				So(math.Abs(compensator[1]-baseline[1]*span), ShouldBeLessThan, 1e-12)
			})
		})

		Convey("When the window has no extent", func() {
			integral, _ := integrate(t, path, decay)
			compensator := compensatorOf(t, baseline, excitation, integral, 0, decay, 2)

			Convey("Then there is no interval to account for and nothing is reported", func() {
				So(compensator, ShouldBeEmpty)
			})
		})

		Convey("When the decay rate is not positive", func() {
			integral, _ := integrate(t, path, decay)
			compensator := compensatorOf(t, baseline, excitation, integral, span, 0, 2)

			Convey("Then the kernel has no scale and nothing is reported", func() {
				So(compensator, ShouldBeEmpty)
			})
		})
	})
}
