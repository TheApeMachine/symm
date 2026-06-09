package causal

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestRidgeSolverSolve(t *testing.T) {
	Convey("Given a well-conditioned normal system", t, func() {
		normal := [][]float64{
			{2, 0.5},
			{0.5, 2},
		}
		vector := []float64{1, 1}
		solver := NewRidgeSolver()

		solution, err := solver.Solve(normal, vector)

		Convey("It should return a finite solution", func() {
			So(err, ShouldBeNil)
			So(len(solution), ShouldEqual, 2)
		})
	})
}
