//go:build !darwin || !cgo

package manifold

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestNewSolver_stub(testingTB *testing.T) {
	Convey("Given the stub solver build", testingTB, func() {
		solver, err := NewSolver(Config{})

		Convey("It should refuse to start without Metal", func() {
			So(solver, ShouldBeNil)
			So(err, ShouldNotBeNil)
		})
	})
}

func TestSolverStub_methods(testingTB *testing.T) {
	Convey("Given a zero-value stub solver", testingTB, func() {
		solver := &Solver{}

		Convey("It should error on runtime operations", func() {
			So(solver.ResetDeposits(), ShouldNotBeNil)
			So(solver.DepositCell(0, 0, 0, 1, 0, 0, 0, 1), ShouldNotBeNil)
			So(solver.SetOscillators(nil), ShouldNotBeNil)

			_, stepErr := solver.Step()

			So(stepErr, ShouldNotBeNil)
		})
	})
}
