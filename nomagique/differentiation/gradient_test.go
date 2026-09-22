package differentiation_test

import (
	"context"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/differentiation"
)

func TestGradient(t *testing.T) {
	Convey("Given a Gradient server and client", t, func() {
		ctx := context.Background()
		server := differentiation.NewGradient(ctx)
		So(server, ShouldNotBeNil)

		client := differentiation.Gradient_ServerToClient(server)
		So(client.IsValid(), ShouldBeTrue)

		Convey("When writing input values", func() {
			err := client.Write(ctx, func(params differentiation.Gradient_write_Params) error {
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
