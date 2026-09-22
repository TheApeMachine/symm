package probability_test

import (
	"context"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/probability"
)

func TestGumbelRight(t *testing.T) {
	Convey("Given a GumbelRight server and client", t, func() {
		ctx := context.Background()
		server := probability.NewGumbelRight(ctx)
		So(server, ShouldNotBeNil)

		client := probability.GumbelRight_ServerToClient(server)
		So(client.IsValid(), ShouldBeTrue)

		Convey("When writing input values", func() {
			err := client.Write(ctx, func(params probability.GumbelRight_write_Params) error {
				params.SetMu(0.0)
				params.SetBeta(1.0)
				params.SetX(1.0)
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
