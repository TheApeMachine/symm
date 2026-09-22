package strategy_test

import (
	"context"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/financial/strategy"
)

func TestOr(t *testing.T) {
	Convey("Given a Or server and client", t, func() {
		ctx := context.Background()
		server := strategy.NewOr(ctx)
		So(server, ShouldNotBeNil)

		client := strategy.Or_ServerToClient(server)
		So(client.IsValid(), ShouldBeTrue)

		Convey("When writing input values", func() {
			err := client.Write(ctx, func(params strategy.Or_write_Params) error {
				params.SetAction1(1)
				params.SetAction2(0)
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
