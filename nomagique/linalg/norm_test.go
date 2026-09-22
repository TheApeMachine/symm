package linalg_test

import (
	"context"
	"testing"
	"gonum.org/v1/gonum/mat"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/linalg"
)

func TestNorm(t *testing.T) {
	Convey("Given a Norm server and client", t, func() {
		ctx := context.Background()
		server := linalg.NewNorm(ctx)
		So(server, ShouldNotBeNil)

		client := linalg.Norm_ServerToClient(server)
		So(client.IsValid(), ShouldBeTrue)

		Convey("When writing input values", func() {
			err := client.Write(ctx, func(params linalg.Norm_write_Params) error {
				matA, err := params.NewA()
				if err != nil { return err }
				denseA := mat.NewDense(2, 2, []float64{2, 1, 1, 3})
				err = linalg.DenseToMatrix(denseA, matA)
				if err != nil { return err }
				params.SetOrd(2.0)
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
