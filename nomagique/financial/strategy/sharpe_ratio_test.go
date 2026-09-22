package strategy_test

import (
	"context"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/financial/strategy"
)

func TestSharpeRatio(t *testing.T) {
	Convey("Given a SharpeRatio server and client", t, func() {
		ctx := context.Background()
		server := strategy.NewSharpeRatio(ctx)
		So(server, ShouldNotBeNil)

		client := strategy.SharpeRatio_ServerToClient(server)
		So(client.IsValid(), ShouldBeTrue)

		Convey("When writing input values", func() {
			err := client.Write(ctx, func(params strategy.SharpeRatio_write_Params) error {
				params.SetOutcome(0.05)
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
