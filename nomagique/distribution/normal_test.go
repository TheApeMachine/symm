package distribution_test

import (
	"context"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/distribution"
)

func TestNormal(t *testing.T) {
	Convey("Given a Normal server and client", t, func() {
		ctx := context.Background()
		server := distribution.NewNormal(ctx)
		So(server, ShouldNotBeNil)

		client := distribution.Normal_ServerToClient(server)
		So(client.IsValid(), ShouldBeTrue)

		Convey("When writing input values", func() {
			err := client.Write(ctx, func(params distribution.Normal_write_Params) error {
				listMu, err := params.NewMu(2)

				if err != nil {
					return err
				}
				listMu.Set(0, 0.0)
				listMu.Set(1, 0.0)
				listSigma, err := params.NewSigma(4)

				if err != nil {
					return err
				}
				listSigma.Set(0, 1.0)
				listSigma.Set(1, 0.0)
				listSigma.Set(2, 0.0)
				listSigma.Set(3, 1.0)
				params.SetDim(2)
				listX, err := params.NewX(2)

				if err != nil {
					return err
				}
				listX.Set(0, 0.5)
				listX.Set(1, 0.5)
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
