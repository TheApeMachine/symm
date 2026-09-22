package probability_test

import (
	"context"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/probability"
)

func TestBinomial(t *testing.T) {
	Convey("Given a Binomial server and client", t, func() {
		ctx := context.Background()
		server := probability.NewBinomial(ctx)
		So(server, ShouldNotBeNil)

		client := probability.Binomial_ServerToClient(server)
		So(client.IsValid(), ShouldBeTrue)

		Convey("When writing input values", func() {
			err := client.Write(ctx, func(params probability.Binomial_write_Params) error {
				params.SetN(10.0)
				params.SetP(0.5)
				params.SetX(5.0)
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
