package optimization_test

import (
	"context"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/optimization"
)

func TestNewton(t *testing.T) {
	Convey("Given a Newton server and client", t, func() {
		ctx := context.Background()
		server := optimization.NewNewton(ctx)
		So(server, ShouldNotBeNil)

		client := optimization.Newton_ServerToClient(server)
		So(client.IsValid(), ShouldBeTrue)

		Convey("When writing input values", func() {
			err := client.Write(ctx, func(params optimization.Newton_write_Params) error {
				listInitX, err := params.NewInitX(2)

				if err != nil {
					return err
				}
				listInitX.Set(0, 0.0)
				listInitX.Set(1, 0.0)
				listMatrixA, err := params.NewMatrixA(4)

				if err != nil {
					return err
				}
				listMatrixA.Set(0, 2.0)
				listMatrixA.Set(1, 0.0)
				listMatrixA.Set(2, 0.0)
				listMatrixA.Set(3, 2.0)
				listVectorB, err := params.NewVectorB(2)

				if err != nil {
					return err
				}
				listVectorB.Set(0, 2.0)
				listVectorB.Set(1, 4.0)
				params.SetDim(2)
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
