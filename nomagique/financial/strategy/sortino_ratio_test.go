package strategy_test

import (
	"context"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/financial/strategy"
)

func TestSortinoRatio(t *testing.T) {
	Convey("Given a SortinoRatio server and client", t, func() {
		ctx := context.Background()
		server := strategy.NewSortinoRatio(ctx)
		So(server, ShouldNotBeNil)

		client := strategy.SortinoRatio_ServerToClient(server)
		So(client.IsValid(), ShouldBeTrue)

		Convey("When writing input values", func() {
			err := client.Write(ctx, func(params strategy.SortinoRatio_write_Params) error {
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
