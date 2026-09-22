package linalg_test

import (
	"context"
	"gonum.org/v1/gonum/mat"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/linalg"
)

func TestVecScale(t *testing.T) {
	Convey("Given a VecScale server and client", t, func() {
		ctx := context.Background()
		server := linalg.NewVecScale(ctx)
		So(server, ShouldNotBeNil)

		client := linalg.VecScale_ServerToClient(server)
		So(client.IsValid(), ShouldBeTrue)

		Convey("When writing input values", func() {
			err := client.Write(ctx, func(params linalg.VecScale_write_Params) error {
				params.SetAlpha(2.0)
				vecU, err := params.NewU()
				if err != nil {
					return err
				}
				denseU := mat.NewVecDense(2, []float64{1, 2})
				err = linalg.VecDenseToVector(denseU, vecU)
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
