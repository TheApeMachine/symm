package optimization_test

import (
	"context"
	"math"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/optimization"
)

/*
quadratic is the objective the optimizer used to carry inside itself:
0.5*x'Ax - b'x with A = diag(2, 2) and b = (2, 4), minimized at (1, 2). It
now lives outside the node, which is the whole point of the interface.
*/
func quadratic(point []float64) (float64, []float64) {
	matrixA := [][]float64{{2, 0}, {0, 2}}
	vectorB := []float64{2, 4}
	value := 0.0
	gradient := make([]float64, len(point))

	for row := range point {
		product := 0.0

		for col := range point {
			product += matrixA[row][col] * point[col]
		}

		value += 0.5*point[row]*product - vectorB[row]*point[row]
		gradient[row] = product - vectorB[row]
	}

	return value, gradient
}

/*
rosenbrock is a curved valley with its minimum at (1, 1). A stepper that
only ever moved along the steepest descent would crawl along its floor; one
that is genuinely accumulating curvature turns the corner.
*/
func rosenbrock(point []float64) (float64, []float64) {
	first := point[1] - point[0]*point[0]
	second := 1 - point[0]
	value := 100*first*first + second*second

	return value, []float64{
		-400*point[0]*first - 2*second,
		200 * first,
	}
}

/*
descend drives the stepper around its loop against one objective, and reports
where it settled and how many evaluations it took.
*/
func descend(
	t *testing.T,
	objective func([]float64) (float64, []float64),
	seed []float64,
	tolerance float64,
	budget int,
) ([]float64, bool, int) {
	t.Helper()
	ctx := context.Background()
	client := optimization.LBFGS_ServerToClient(optimization.NewLBFGS(ctx))
	point := make([]float64, len(seed))
	value := 0.0
	gradient := make([]float64, len(seed))
	converged := false
	evaluations := 0

	for step := 0; step < budget && !converged; step++ {
		err := client.Write(ctx, func(params optimization.LBFGS_write_Params) error {
			list, err := params.NewGradient(int32(len(gradient)))

			if err != nil {
				return err
			}

			for index, item := range gradient {
				list.Set(index, item)
			}

			start, err := params.NewSeed(int32(len(seed)))

			if err != nil {
				return err
			}

			for index, item := range seed {
				start.Set(index, item)
			}

			params.SetFVal(value)
			params.SetMemory(8)
			params.SetTolerance(tolerance)
			return nil
		})

		if err != nil {
			t.Fatalf("lbfgs write: %v", err)
		}

		if err := client.WaitStreaming(); err != nil {
			t.Fatalf("lbfgs stream: %v", err)
		}

		future, release := client.Done(ctx, nil)
		results, err := future.Struct()

		if err != nil {
			t.Fatalf("lbfgs done: %v", err)
		}

		listX, err := results.X()

		if err != nil {
			t.Fatalf("lbfgs x: %v", err)
		}

		point = make([]float64, listX.Len())

		for index := range point {
			point[index] = listX.At(index)
		}

		converged = results.Converged()
		release()
		value, gradient = objective(point)
		evaluations++
	}

	return point, converged, evaluations
}

func TestLBFGSServer_Write(t *testing.T) {
	Convey("Given a stepper driven by an objective it knows nothing about", t, func() {
		Convey("When it descends a quadratic", func() {
			point, converged, _ := descend(t, quadratic, []float64{0, 0}, 1e-9, 200)

			Convey("Then it settles on the analytic minimum", func() {
				So(converged, ShouldBeTrue)
				So(math.Abs(point[0]-1), ShouldBeLessThan, 1e-6)
				So(math.Abs(point[1]-2), ShouldBeLessThan, 1e-6)
			})
		})

		Convey("When it descends a curved valley", func() {
			point, converged, _ := descend(t, rosenbrock, []float64{-1.2, 1}, 1e-8, 5000)

			Convey("Then the curvature it accumulated carries it to the minimum", func() {
				So(converged, ShouldBeTrue)
				So(math.Abs(point[0]-1), ShouldBeLessThan, 1e-4)
				So(math.Abs(point[1]-1), ShouldBeLessThan, 1e-4)
			})
		})

		Convey("When it is started on the answer", func() {
			point, converged, evaluations := descend(t, quadratic, []float64{1, 2}, 1e-9, 200)

			Convey("Then it recognises there is nowhere to go", func() {
				So(converged, ShouldBeTrue)
				So(math.Abs(point[0]-1), ShouldBeLessThan, 1e-12)
				So(evaluations, ShouldBeLessThan, 5)
			})
		})

		Convey("When the valley is crossed", func() {
			_, converged, evaluations := descend(t, rosenbrock, []float64{-1.2, 1}, 1e-8, 5000)

			Convey("Then it takes the number of evaluations curvature buys", func() {
				// Steepest descent alone grinds along this valley floor for
				// thousands of evaluations. Staying well inside that is the
				// observable difference the retained history makes.
				So(converged, ShouldBeTrue)
				So(evaluations, ShouldBeLessThan, 500)
			})
		})
	})
}
