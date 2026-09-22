package differentiation_test

import (
	"context"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/differentiation"
)

func TestDerivative(t *testing.T) {
	Convey("Given a Derivative server and client", t, func() {
		ctx := context.Background()
		server := differentiation.NewDerivative(ctx)
		So(server, ShouldNotBeNil)

		client := differentiation.Derivative_ServerToClient(server)
		So(client.IsValid(), ShouldBeTrue)

		Convey("When writing input values", func() {
			err := client.Write(ctx, func(params differentiation.Derivative_write_Params) error {
				listCoeffs, err := params.NewCoeffs(3)

				if err != nil {
					return err
				}
				listCoeffs.Set(0, 1.0)
				listCoeffs.Set(1, 2.0)
				listCoeffs.Set(2, 3.0)
				params.SetX(2.0)
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
