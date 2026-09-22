package differentiation_test

import (
	"context"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/differentiation"
)

func TestHessian(t *testing.T) {
	Convey("Given a Hessian server and client", t, func() {
		ctx := context.Background()
		server := differentiation.NewHessian(ctx)
		So(server, ShouldNotBeNil)

		client := differentiation.Hessian_ServerToClient(server)
		So(client.IsValid(), ShouldBeTrue)

		Convey("When writing input values", func() {
			err := client.Write(ctx, func(params differentiation.Hessian_write_Params) error {
				listCoeffs, err := params.NewCoeffs(2)

				if err != nil {
					return err
				}
				listCoeffs.Set(0, 1.0)
				listCoeffs.Set(1, 2.0)
				listX, err := params.NewX(2)

				if err != nil {
					return err
				}
				listX.Set(0, 1.0)
				listX.Set(1, 1.0)
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
