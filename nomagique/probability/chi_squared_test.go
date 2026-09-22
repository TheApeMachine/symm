package probability_test

import (
	"context"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/probability"
)

func TestChiSquared(t *testing.T) {
	Convey("Given a ChiSquared server and client", t, func() {
		ctx := context.Background()
		server := probability.NewChiSquared(ctx)
		So(server, ShouldNotBeNil)

		client := probability.ChiSquared_ServerToClient(server)
		So(client.IsValid(), ShouldBeTrue)

		Convey("When writing input values", func() {
			err := client.Write(ctx, func(params probability.ChiSquared_write_Params) error {
				params.SetK(3.0)
				params.SetX(1.5)
				params.SetP(0.5)
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
