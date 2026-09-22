package distribution_test

import (
	"context"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/distribution"
)

func TestUniform(t *testing.T) {
	Convey("Given a Uniform server and client", t, func() {
		ctx := context.Background()
		server := distribution.NewUniform(ctx)
		So(server, ShouldNotBeNil)

		client := distribution.Uniform_ServerToClient(server)
		So(client.IsValid(), ShouldBeTrue)

		Convey("When writing input values", func() {
			err := client.Write(ctx, func(params distribution.Uniform_write_Params) error {
				listMin, err := params.NewMin(2)

				if err != nil {
					return err
				}
				listMin.Set(0, 0.0)
				listMin.Set(1, 0.0)
				listMax, err := params.NewMax(2)

				if err != nil {
					return err
				}
				listMax.Set(0, 1.0)
				listMax.Set(1, 2.0)
				params.SetDim(2)
				listX, err := params.NewX(2)

				if err != nil {
					return err
				}
				listX.Set(0, 0.5)
				listX.Set(1, 1.0)
				listP, err := params.NewP(2)

				if err != nil {
					return err
				}
				listP.Set(0, 0.5)
				listP.Set(1, 0.5)
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
