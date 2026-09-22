package integration_test

import (
	"context"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/integration"
)

func TestSimpsons(t *testing.T) {
	Convey("Given a Simpsons server and client", t, func() {
		ctx := context.Background()
		server := integration.NewSimpsons(ctx)
		So(server, ShouldNotBeNil)

		client := integration.Simpsons_ServerToClient(server)
		So(client.IsValid(), ShouldBeTrue)

		Convey("When writing input values", func() {
			err := client.Write(ctx, func(params integration.Simpsons_write_Params) error {
				listX, err := params.NewX(4)

				if err != nil {
					return err
				}
				listX.Set(0, 0.0)
				listX.Set(1, 1.0)
				listX.Set(2, 2.0)
				listX.Set(3, 3.0)
				listY, err := params.NewY(4)

				if err != nil {
					return err
				}
				listY.Set(0, 0.0)
				listY.Set(1, 1.0)
				listY.Set(2, 4.0)
				listY.Set(3, 9.0)
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
