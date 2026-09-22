package distribution_test

import (
	"context"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/distribution"
)

func TestUniformKullbackLeibler(t *testing.T) {
	Convey("Given a UniformKullbackLeibler server and client", t, func() {
		ctx := context.Background()
		server := distribution.NewUniformKullbackLeibler(ctx)
		So(server, ShouldNotBeNil)

		client := distribution.UniformKullbackLeibler_ServerToClient(server)
		So(client.IsValid(), ShouldBeTrue)

		Convey("When writing input values", func() {
			err := client.Write(ctx, func(params distribution.UniformKullbackLeibler_write_Params) error {
				listMinL, err := params.NewMinL(2)

				if err != nil {
					return err
				}
				listMinL.Set(0, 0.0)
				listMinL.Set(1, 0.0)
				listMaxL, err := params.NewMaxL(2)

				if err != nil {
					return err
				}
				listMaxL.Set(0, 1.0)
				listMaxL.Set(1, 1.0)
				listMinR, err := params.NewMinR(2)

				if err != nil {
					return err
				}
				listMinR.Set(0, 0.0)
				listMinR.Set(1, 0.0)
				listMaxR, err := params.NewMaxR(2)

				if err != nil {
					return err
				}
				listMaxR.Set(0, 2.0)
				listMaxR.Set(1, 2.0)
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
