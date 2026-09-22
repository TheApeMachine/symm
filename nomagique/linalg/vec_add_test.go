package linalg_test

import (
	"context"
	"testing"
	"gonum.org/v1/gonum/mat"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/linalg"
)

func TestVecAdd(t *testing.T) {
	Convey("Given a VecAdd server and client", t, func() {
		ctx := context.Background()
		server := linalg.NewVecAdd(ctx)
		So(server, ShouldNotBeNil)

		client := linalg.VecAdd_ServerToClient(server)
		So(client.IsValid(), ShouldBeTrue)

		Convey("When writing input values", func() {
			err := client.Write(ctx, func(params linalg.VecAdd_write_Params) error {
				vecU, err := params.NewU()
				if err != nil { return err }
				denseU := mat.NewVecDense(2, []float64{1, 2})
				err = linalg.VecDenseToVector(denseU, vecU)
				if err != nil { return err }
				vecV, err := params.NewV()
				if err != nil { return err }
				denseV := mat.NewVecDense(2, []float64{1, 2})
				err = linalg.VecDenseToVector(denseV, vecV)
				if err != nil { return err }
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
