package hawkes_test

import (
	"context"
	"math"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/statistic/hawkes"
)

/*
stationaryOf drives the StationaryIntensity node over one parameter set.
*/
func stationaryOf(t *testing.T, baseline, branching []float64, dimension int) ([]float64, bool) {
	t.Helper()
	ctx := context.Background()
	client := hawkes.StationaryIntensity_ServerToClient(hawkes.NewStationaryIntensity())

	err := client.Write(ctx, func(params hawkes.StationaryIntensity_write_Params) error {
		if err := writeFloats(params.NewBaseline, baseline); err != nil {
			return err
		}

		if err := writeFloats(params.NewBranching, branching); err != nil {
			return err
		}

		params.SetDimension(int32(dimension))
		return nil
	})

	if err != nil {
		t.Fatalf("stationary intensity write: %v", err)
	}

	if err := client.WaitStreaming(); err != nil {
		t.Fatalf("stationary intensity stream: %v", err)
	}

	future, release := client.Done(ctx, nil)
	defer release()
	results, err := future.Struct()

	if err != nil {
		t.Fatalf("stationary intensity done: %v", err)
	}

	list, err := results.Intensity()

	if err != nil {
		t.Fatalf("stationary intensity list: %v", err)
	}

	return readList(list.Len(), list.At), results.Defined()
}

func TestStationaryIntensityServer_Write(t *testing.T) {
	Convey("Given a subcritical process", t, func() {
		baseline := []float64{0.5, 0.3}
		excitation := []float64{0.6, 0.2, 0.3, 0.5}
		decay := 1.5
		branching := branchingOf(t, excitation, decay, 2)

		Convey("When the long-run intensity is solved for", func() {
			intensity, defined := stationaryOf(t, baseline, branching, 2)

			Convey("Then it is defined", func() {
				So(defined, ShouldBeTrue)
				So(len(intensity), ShouldEqual, 2)
			})

			Convey("Then it satisfies the equation it claims to solve", func() {
				for row := 0; row < 2; row++ {
					residual := intensity[row]

					for column := 0; column < 2; column++ {
						residual -= branching[row*2+column] * intensity[column]
					}

					So(math.Abs(residual-baseline[row]), ShouldBeLessThan, 1e-12)
				}
			})

			Convey("Then it exceeds the baseline, because excitation adds arrivals", func() {
				So(intensity[0], ShouldBeGreaterThan, baseline[0])
				So(intensity[1], ShouldBeGreaterThan, baseline[1])
			})

			Convey("Then a simulated path arrives at that rate", func() {
				// The only check here that could catch the solve being
				// right for the wrong process: run the process and count.
				horizon := 20000.0
				path := simulate(2, baseline, excitation, decay, horizon, 20260922)
				observed := make([]float64, 2)

				for _, component := range path.components {
					observed[int(component)]++
				}

				for index := range observed {
					rate := observed[index] / horizon
					So(math.Abs(rate-intensity[index]), ShouldBeLessThan, 0.05*intensity[index])
				}
			})
		})

		Convey("When the process is critical", func() {
			_, defined := stationaryOf(t, baseline, []float64{0.5, 0.5, 0.5, 0.5}, 2)

			Convey("Then no long-run rate exists and none is reported", func() {
				So(defined, ShouldBeFalse)
			})
		})

		Convey("When excitation would drive the solution negative", func() {
			_, defined := stationaryOf(t, baseline, []float64{1.4, 0.1, 0.1, 1.4}, 2)

			Convey("Then a negative rate is refused rather than returned", func() {
				So(defined, ShouldBeFalse)
			})
		})

		Convey("When there is no excitation at all", func() {
			intensity, defined := stationaryOf(t, baseline, []float64{0, 0, 0, 0}, 2)

			Convey("Then each component runs at its own baseline", func() {
				So(defined, ShouldBeTrue)
				So(math.Abs(intensity[0]-baseline[0]), ShouldBeLessThan, 1e-12)
				So(math.Abs(intensity[1]-baseline[1]), ShouldBeLessThan, 1e-12)
			})
		})
	})
}
