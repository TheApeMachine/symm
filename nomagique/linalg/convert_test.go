package linalg_test

import (
	"testing"

	capnp "capnproto.org/go/capnp/v3"
	"gonum.org/v1/gonum/mat"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/linalg"
)

func TestConvert(t *testing.T) {
	Convey("Given matrix and vector conversion utilities", t, func() {
		Convey("When converting a valid *mat.Dense to Cap'n Proto Matrix and back", func() {
			source := mat.NewDense(2, 3, []float64{1, 2, 3, 4, 5, 6})

			_, seg, err := capnp.NewMessage(capnp.SingleSegment(nil))
			So(err, ShouldBeNil)

			capMatrix, err := linalg.NewMatrix(seg)
			So(err, ShouldBeNil)

			err = linalg.DenseToMatrix(source, capMatrix)
			So(err, ShouldBeNil)
			So(capMatrix.Rows(), ShouldEqual, 2)
			So(capMatrix.Cols(), ShouldEqual, 3)

			restored, err := linalg.MatrixToDense(capMatrix)
			So(err, ShouldBeNil)
			So(mat.Equal(source, restored), ShouldBeTrue)
		})

		Convey("When converting a valid *mat.VecDense to Cap'n Proto Vector and back", func() {
			source := mat.NewVecDense(4, []float64{10, 20, 30, 40})

			_, seg, err := capnp.NewMessage(capnp.SingleSegment(nil))
			So(err, ShouldBeNil)

			capVec, err := linalg.NewVector(seg)
			So(err, ShouldBeNil)

			err = linalg.VecDenseToVector(source, capVec)
			So(err, ShouldBeNil)

			restored, err := linalg.VectorToVecDense(capVec)
			So(err, ShouldBeNil)
			So(restored.Len(), ShouldEqual, 4)
			So(restored.AtVec(0), ShouldEqual, 10)
			So(restored.AtVec(3), ShouldEqual, 40)
		})
	})
}
