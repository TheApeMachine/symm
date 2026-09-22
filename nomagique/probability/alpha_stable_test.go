package probability_test

import (
	"context"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/probability"
)

func TestAlphaStable(t *testing.T) {
	Convey("Given a AlphaStable server and client", t, func() {
		ctx := context.Background()
		server := probability.NewAlphaStable(ctx)
		So(server, ShouldNotBeNil)

		client := probability.AlphaStable_ServerToClient(server)
		So(client.IsValid(), ShouldBeTrue)

		Convey("When writing input values", func() {
			err := client.Write(ctx, func(params probability.AlphaStable_write_Params) error {
				params.SetAlpha(1.5)
				params.SetBeta(0.0)
				params.SetC(1.0)
				params.SetMu(0.0)
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
