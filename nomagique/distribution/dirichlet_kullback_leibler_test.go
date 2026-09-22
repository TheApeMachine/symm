package distribution_test

import (
	"context"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/distribution"
)

func TestDirichletKullbackLeibler(t *testing.T) {
	Convey("Given a DirichletKullbackLeibler server and client", t, func() {
		ctx := context.Background()
		server := distribution.NewDirichletKullbackLeibler(ctx)
		So(server, ShouldNotBeNil)

		client := distribution.DirichletKullbackLeibler_ServerToClient(server)
		So(client.IsValid(), ShouldBeTrue)

		Convey("When writing input values", func() {
			err := client.Write(ctx, func(params distribution.DirichletKullbackLeibler_write_Params) error {
				listAlphaL, err := params.NewAlphaL(3)

				if err != nil {
					return err
				}
				listAlphaL.Set(0, 2.0)
				listAlphaL.Set(1, 3.0)
				listAlphaL.Set(2, 5.0)
				listAlphaR, err := params.NewAlphaR(3)

				if err != nil {
					return err
				}
				listAlphaR.Set(0, 1.0)
				listAlphaR.Set(1, 1.0)
				listAlphaR.Set(2, 1.0)
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
