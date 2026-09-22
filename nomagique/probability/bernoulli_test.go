package probability_test

import (
	"context"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/probability"
)

func TestBernoulli(t *testing.T) {
	Convey("Given a Bernoulli server and client", t, func() {
		ctx := context.Background()
		server := probability.NewBernoulli(ctx)
		So(server, ShouldNotBeNil)

		client := probability.Bernoulli_ServerToClient(server)
		So(client.IsValid(), ShouldBeTrue)

		Convey("When writing input values", func() {
			err := client.Write(ctx, func(params probability.Bernoulli_write_Params) error {
				params.SetP(0.6)
				params.SetX(1.0)
				params.SetQ(0.5)
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
