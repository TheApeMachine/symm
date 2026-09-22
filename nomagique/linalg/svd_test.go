package linalg_test

import (
	"context"
	"gonum.org/v1/gonum/mat"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/linalg"
)

func TestSVD(t *testing.T) {
	Convey("Given a SVD server and client", t, func() {
		ctx := context.Background()
		server := linalg.NewSVD(ctx)
		So(server, ShouldNotBeNil)

		client := linalg.SVD_ServerToClient(server)
		So(client.IsValid(), ShouldBeTrue)

		Convey("When writing input values", func() {
			err := client.Write(ctx, func(params linalg.SVD_write_Params) error {
				matA, err := params.NewA()
				if err != nil {
					return err
				}
				denseA := mat.NewDense(2, 2, []float64{2, 1, 1, 3})
				err = linalg.DenseToMatrix(denseA, matA)
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
