package hawkes_test

import (
	"context"
	"math"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/statistic/hawkes"
)

/*
mapped is what the Parameters node makes of one point in the optimizer's
coordinates.
*/
type mapped struct {
	baseline   []float64
	excitation []float64
	decay      float64
	jacobian   []float64
}

/*
mapCoordinates drives the Parameters node over one point and its bounds.
*/
func mapCoordinates(t *testing.T, coordinates, lower, upper []float64, dimension int) mapped {
	t.Helper()
	ctx := context.Background()
	client := hawkes.Parameters_ServerToClient(hawkes.NewParameters())

	err := client.Write(ctx, func(params hawkes.Parameters_write_Params) error {
		if err := writeFloats(params.NewCoordinates, coordinates); err != nil {
			return err
		}

		if err := writeFloats(params.NewLower, lower); err != nil {
			return err
		}

		if err := writeFloats(params.NewUpper, upper); err != nil {
			return err
		}

		params.SetDimension(int32(dimension))
		return nil
	})

	if err != nil {
		t.Fatalf("parameters write: %v", err)
	}

	if err := client.WaitStreaming(); err != nil {
		t.Fatalf("parameters stream: %v", err)
	}

	future, release := client.Done(ctx, nil)
	defer release()
	results, err := future.Struct()

	if err != nil {
		t.Fatalf("parameters done: %v", err)
	}

	baseline, err := results.Baseline()

	if err != nil {
		t.Fatalf("parameters baseline: %v", err)
	}

	excitation, err := results.Excitation()

	if err != nil {
		t.Fatalf("parameters excitation: %v", err)
	}

	jacobian, err := results.Jacobian()

	if err != nil {
		t.Fatalf("parameters jacobian: %v", err)
	}

	return mapped{
		baseline:   readList(baseline.Len(), baseline.At),
		excitation: readList(excitation.Len(), excitation.At),
		decay:      results.Decay(),
		jacobian:   readList(jacobian.Len(), jacobian.At),
	}
}

/*
uniform builds bounds repeating one log-space interval across every
coordinate.
*/
func uniform(width int, low, high float64) ([]float64, []float64) {
	lower := make([]float64, width)
	upper := make([]float64, width)

	for index := range lower {
		lower[index] = math.Log(low)
		upper[index] = math.Log(high)
	}

	return lower, upper
}

func TestParametersServer_Write(t *testing.T) {
	Convey("Given bounds on a two-component process", t, func() {
		dimension := 2
		width := dimension + dimension*dimension + 1
		lower, upper := uniform(width, 0.01, 4.0)

		Convey("When a point is mapped", func() {
			coordinates := []float64{-1.2, 0.4, 0.9, -2.0, 1.5, 0.1, 0.7}
			result := mapCoordinates(t, coordinates, lower, upper, dimension)

			Convey("Then it is laid out as the baselines, the matrix, then the decay", func() {
				So(len(result.baseline), ShouldEqual, 2)
				So(len(result.excitation), ShouldEqual, 4)
				So(len(result.jacobian), ShouldEqual, width)
				So(result.decay, ShouldBeGreaterThan, 0)
			})

			Convey("Then every parameter lands strictly inside its bounds", func() {
				// The bounds are stated in log space, so the comparison is
				// against what they exponentiate to rather than the decimal
				// they were written as: the round trip is not exact.
				low := math.Exp(lower[0])
				high := math.Exp(upper[0])

				for _, value := range flatten(result, dimension) {
					So(value, ShouldBeGreaterThan, low)
					So(value, ShouldBeLessThan, high)
				}
			})

			Convey("Then each jacobian entry matches a central difference of the map", func() {
				// The jacobian is what carries a gradient back into the
				// coordinates an optimizer moves in, so it has to be the
				// actual derivative of this map, not an approximation of a
				// different one.
				step := 1e-6

				for index := 0; index < width; index++ {
					up := mapCoordinates(t, perturb(coordinates, index, step), lower, upper, dimension)
					down := mapCoordinates(t, perturb(coordinates, index, -step), lower, upper, dimension)
					numeric := (flatten(up, dimension)[index] - flatten(down, dimension)[index]) / (2 * step)

					So(math.Abs(numeric-result.jacobian[index]), ShouldBeLessThan, 1e-5*math.Max(1, math.Abs(numeric)))
				}
			})
		})

		Convey("When a coordinate runs to either extreme", func() {
			far := make([]float64, width)
			near := make([]float64, width)

			for index := range far {
				far[index] = 400
				near[index] = -400
			}

			high := mapCoordinates(t, far, lower, upper, dimension)
			low := mapCoordinates(t, near, lower, upper, dimension)

			Convey("Then it saturates on that bound rather than passing it", func() {
				// Far enough out, the softplus ratio reaches 0 or 1 in
				// double precision and the parameter pins to the bound
				// exactly. What matters is that it never crosses: an
				// optimizer taking a wild step must not be handed a decay
				// the window cannot resolve.
				ceiling := math.Exp(upper[width-1])
				floor := math.Exp(lower[width-1])

				So(high.decay, ShouldBeLessThanOrEqualTo, ceiling)
				So(high.decay, ShouldBeGreaterThan, 0.9*ceiling)
				So(low.decay, ShouldBeGreaterThanOrEqualTo, floor)
				So(low.decay, ShouldBeLessThan, 1.1*floor)
			})

			Convey("Then no parameter is ever negative, whatever the search proposes", func() {
				So(low.baseline[0], ShouldBeGreaterThan, 0)
				So(high.baseline[0], ShouldBeGreaterThan, 0)
			})
		})

		Convey("When the map is walked along one coordinate", func() {
			previous := 0.0

			Convey("Then it increases without ever turning back", func() {
				for position := -6.0; position <= 6.0; position += 0.5 {
					coordinates := make([]float64, width)
					coordinates[width-1] = position
					result := mapCoordinates(t, coordinates, lower, upper, dimension)
					So(result.decay, ShouldBeGreaterThan, previous)
					previous = result.decay
				}
			})
		})

		Convey("When a bound is degenerate", func() {
			flat := make([]float64, width)
			pinned, _ := uniform(width, 2.0, 2.0)
			result := mapCoordinates(t, flat, pinned, pinned, dimension)

			Convey("Then the parameter is pinned and cannot be moved", func() {
				So(math.Abs(result.decay-2.0), ShouldBeLessThan, 1e-12)
				So(result.jacobian[width-1], ShouldEqual, 0)
			})
		})

		Convey("When fewer coordinates arrive than the dimension needs", func() {
			result := mapCoordinates(t, []float64{0.1, 0.2}, lower, upper, dimension)

			Convey("Then nothing is reported rather than a partial parameter set", func() {
				So(result.baseline, ShouldBeEmpty)
				So(result.excitation, ShouldBeEmpty)
				So(result.decay, ShouldEqual, 0)
			})
		})
	})
}

/*
flatten lays a mapped parameter set back out in coordinate order, so a
difference can be taken against the jacobian entry by entry.
*/
func flatten(result mapped, dimension int) []float64 {
	values := make([]float64, 0, dimension+dimension*dimension+1)
	values = append(values, result.baseline...)
	values = append(values, result.excitation...)
	return append(values, result.decay)
}
