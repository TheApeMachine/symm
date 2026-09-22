package distribution_test

import (
	"context"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/distribution"
)

func TestDirichlet(t *testing.T) {
	Convey("Given a Dirichlet server and client", t, func() {
		ctx := context.Background()
		server := distribution.NewDirichlet(ctx)
		So(server, ShouldNotBeNil)

		client := distribution.Dirichlet_ServerToClient(server)
		So(client.IsValid(), ShouldBeTrue)

		Convey("When writing input values", func() {
			err := client.Write(ctx, func(params distribution.Dirichlet_write_Params) error {
				listAlpha, err := params.NewAlpha(3)

				if err != nil {
					return err
				}
				listAlpha.Set(0, 2.0)
				listAlpha.Set(1, 3.0)
				listAlpha.Set(2, 5.0)
				listX, err := params.NewX(3)

				if err != nil {
					return err
				}
				listX.Set(0, 0.2)
				listX.Set(1, 0.3)
				listX.Set(2, 0.5)
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
