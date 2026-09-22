package distribution_test

import (
	"context"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/distribution"
)

func TestNormalLogProb(t *testing.T) {
	Convey("Given a NormalLogProb server and client", t, func() {
		ctx := context.Background()
		server := distribution.NewNormalLogProb(ctx)
		So(server, ShouldNotBeNil)

		client := distribution.NormalLogProb_ServerToClient(server)
		So(client.IsValid(), ShouldBeTrue)

		Convey("When writing input values", func() {
			err := client.Write(ctx, func(params distribution.NormalLogProb_write_Params) error {
				listX, err := params.NewX(2)

				if err != nil {
					return err
				}
				listX.Set(0, 0.5)
				listX.Set(1, 0.5)
				listMu, err := params.NewMu(2)

				if err != nil {
					return err
				}
				listMu.Set(0, 0.0)
				listMu.Set(1, 0.0)
				listCholData, err := params.NewCholData(4)

				if err != nil {
					return err
				}
				listCholData.Set(0, 1.0)
				listCholData.Set(1, 0.0)
				listCholData.Set(2, 0.0)
				listCholData.Set(3, 1.0)
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
