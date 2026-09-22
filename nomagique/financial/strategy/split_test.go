package strategy_test

import (
	"context"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/financial/strategy"
)

func TestSplit(t *testing.T) {
	Convey("Given a Split server and client", t, func() {
		ctx := context.Background()
		server := strategy.NewSplit(ctx)
		So(server, ShouldNotBeNil)

		client := strategy.Split_ServerToClient(server)
		So(client.IsValid(), ShouldBeTrue)

		Convey("When writing input values", func() {
			err := client.Write(ctx, func(params strategy.Split_write_Params) error {
				params.SetBuyAction(1)
				params.SetSellAction(0)
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
