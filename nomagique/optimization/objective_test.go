package optimization_test

import (
	"context"
	"math"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/optimization"
)

/*
present drives the Objective node over one measured value and gradient.
*/
func present(t *testing.T, value float64, gradient, jacobian []float64, sense float64) (float64, []float64) {
	t.Helper()
	ctx := context.Background()
	client := optimization.Objective_ServerToClient(optimization.NewObjective())

	err := client.Write(ctx, func(params optimization.Objective_write_Params) error {
		list, err := params.NewGradient(int32(len(gradient)))

		if err != nil {
			return err
		}

		for index, item := range gradient {
			list.Set(index, item)
		}

		carried, err := params.NewJacobian(int32(len(jacobian)))

		if err != nil {
			return err
		}

		for index, item := range jacobian {
			carried.Set(index, item)
		}

		params.SetValue(value)
		params.SetSense(sense)
		return nil
	})

	if err != nil {
		t.Fatalf("objective write: %v", err)
	}

	if err := client.WaitStreaming(); err != nil {
		t.Fatalf("objective stream: %v", err)
	}

	future, release := client.Done(ctx, nil)
	defer release()
	results, err := future.Struct()

	if err != nil {
		t.Fatalf("objective done: %v", err)
	}

	list, err := results.Gradient()

	if err != nil {
		t.Fatalf("objective gradient: %v", err)
	}

	carried := make([]float64, list.Len())

	for index := range carried {
		carried[index] = list.At(index)
	}

	return results.FVal(), carried
}

func TestObjectiveServer_Write(t *testing.T) {
	Convey("Given a measured value and its gradient", t, func() {
		value := 12.5
		gradient := []float64{2, -3, 0.5}

		Convey("When it is presented to a minimizer as something to maximize", func() {
			presented, carried := present(t, value, gradient, nil, -1)

			Convey("Then both the value and every direction are reversed", func() {
				So(presented, ShouldEqual, -12.5)
				So(carried, ShouldResemble, []float64{-2, 3, -0.5})
			})
		})

		Convey("When it is presented as something to minimize directly", func() {
			presented, carried := present(t, value, gradient, nil, 1)

			Convey("Then it passes through untouched", func() {
				So(presented, ShouldEqual, 12.5)
				So(carried, ShouldResemble, gradient)
			})
		})

		Convey("When the search moves in different coordinates", func() {
			jacobian := []float64{10, 0.5, 2}
			_, carried := present(t, value, gradient, jacobian, -1)

			Convey("Then each direction is carried through its own scale", func() {
				// A gradient handed over without the jacobian would point
				// along the model's parameters, not the coordinates the
				// search actually steps in.
				So(carried[0], ShouldEqual, -20)
				So(carried[1], ShouldEqual, 1.5)
				So(carried[2], ShouldEqual, -1)
			})
		})

		Convey("When a coordinate is pinned by a degenerate bound", func() {
			_, carried := present(t, value, gradient, []float64{10, 0, 2}, -1)

			Convey("Then no step is reported along it", func() {
				So(carried[1], ShouldEqual, 0)
			})
		})

		Convey("When the jacobian is shorter than the gradient", func() {
			_, carried := present(t, value, gradient, []float64{10}, -1)

			Convey("Then the uncovered directions keep the coordinates they arrived in", func() {
				So(carried[0], ShouldEqual, -20)
				So(math.Abs(carried[1]-3), ShouldBeLessThan, 1e-12)
				So(math.Abs(carried[2]+0.5), ShouldBeLessThan, 1e-12)
			})
		})
	})
}
