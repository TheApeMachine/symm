package probability_test

import (
	"context"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/probability"
)

func TestBetaHellinger(t *testing.T) {
	Convey("Given a BetaHellinger server and client", t, func() {
		ctx := context.Background()
		server := probability.NewBetaHellinger(ctx)
		So(server, ShouldNotBeNil)

		client := probability.BetaHellinger_ServerToClient(server)
		So(client.IsValid(), ShouldBeTrue)

		Convey("When writing input values", func() {
			err := client.Write(ctx, func(params probability.BetaHellinger_write_Params) error {
				params.SetAlphaL(2.0)
				params.SetBetaL(2.0)
				params.SetAlphaR(1.0)
				params.SetBetaR(1.0)
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
