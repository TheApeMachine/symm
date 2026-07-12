package manifold

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

type fieldSolverStub struct {
	rho [][]float64
	err error
}

func (stub fieldSolverStub) ReadRhoProjection() ([][]float64, error) {
	return stub.rho, stub.err
}

func TestFieldSnapshotRead(t *testing.T) {
	Convey("Given the solver has completed a density projection", t, func() {
		projection := [][]float64{{1, 2}, {3, 4}}
		field := FieldSnapshot{}

		Convey("It should retain the raw solver matrix", func() {
			So(field.Read(fieldSolverStub{rho: projection}), ShouldBeNil)
			So(field.Rho, ShouldResemble, projection)
		})
	})
}
