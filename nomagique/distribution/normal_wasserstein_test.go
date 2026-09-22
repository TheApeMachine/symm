package distribution_test

import (
	"context"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/distribution"
)

func TestNormalWasserstein(t *testing.T) {
	Convey("Given a NormalWasserstein server and client", t, func() {
		ctx := context.Background()
		server := distribution.NewNormalWasserstein(ctx)
		So(server, ShouldNotBeNil)

		client := distribution.NormalWasserstein_ServerToClient(server)
		So(client.IsValid(), ShouldBeTrue)

		Convey("When writing input values", func() {
			err := client.Write(ctx, func(params distribution.NormalWasserstein_write_Params) error {
				listMuL, err := params.NewMuL(2)

				if err != nil {
					return err
				}
				listMuL.Set(0, 0.0)
				listMuL.Set(1, 0.0)
				listSigmaL, err := params.NewSigmaL(4)

				if err != nil {
					return err
				}
				listSigmaL.Set(0, 1.0)
				listSigmaL.Set(1, 0.0)
				listSigmaL.Set(2, 0.0)
				listSigmaL.Set(3, 1.0)
				listMuR, err := params.NewMuR(2)

				if err != nil {
					return err
				}
				listMuR.Set(0, 1.0)
				listMuR.Set(1, 1.0)
				listSigmaR, err := params.NewSigmaR(4)

				if err != nil {
					return err
				}
				listSigmaR.Set(0, 1.0)
				listSigmaR.Set(1, 0.0)
				listSigmaR.Set(2, 0.0)
				listSigmaR.Set(3, 1.0)
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
