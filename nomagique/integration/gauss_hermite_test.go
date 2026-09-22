package integration_test

import (
	"context"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/integration"
)

func TestGaussHermite(t *testing.T) {
	Convey("Given a GaussHermite server and client", t, func() {
		ctx := context.Background()
		server := integration.NewGaussHermite(ctx)
		So(server, ShouldNotBeNil)

		client := integration.GaussHermite_ServerToClient(server)
		So(client.IsValid(), ShouldBeTrue)

		Convey("When writing input values", func() {
			err := client.Write(ctx, func(params integration.GaussHermite_write_Params) error {
				listCoeffs, err := params.NewCoeffs(3)

				if err != nil {
					return err
				}
				listCoeffs.Set(0, 1.0)
				listCoeffs.Set(1, 0.0)
				listCoeffs.Set(2, 1.0)
				params.SetPoints(10)
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
