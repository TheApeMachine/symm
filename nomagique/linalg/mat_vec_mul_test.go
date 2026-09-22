package linalg_test

import (
	"context"
	"gonum.org/v1/gonum/mat"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/linalg"
)

func TestMatVecMul(t *testing.T) {
	Convey("Given a MatVecMul server and client", t, func() {
		ctx := context.Background()
		server := linalg.NewMatVecMul(ctx)
		So(server, ShouldNotBeNil)

		client := linalg.MatVecMul_ServerToClient(server)
		So(client.IsValid(), ShouldBeTrue)

		Convey("When writing input values", func() {
			err := client.Write(ctx, func(params linalg.MatVecMul_write_Params) error {
				matA, err := params.NewA()
				if err != nil {
					return err
				}
				denseA := mat.NewDense(2, 2, []float64{2, 1, 1, 3})
				err = linalg.DenseToMatrix(denseA, matA)
				if err != nil {
					return err
				}
				vecX, err := params.NewX()
				if err != nil {
					return err
				}
				denseX := mat.NewVecDense(2, []float64{1, 2})
				err = linalg.VecDenseToVector(denseX, vecX)
				if err != nil {
					return err
				}
				return nil
			})
			So(err, ShouldBeNil)

			err = client.WaitStreaming()
			So(err, ShouldBeNil)

			future, release := client.Done(ctx, nil)
			defer release()

			results, err := future.Struct()
			So(err, ShouldBeNil)
			So(results.IsValid(), ShouldBeTrue)
		})
	})
}
