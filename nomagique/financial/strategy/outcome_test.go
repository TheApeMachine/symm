package strategy_test

import (
	"context"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/financial/strategy"
)

func TestOutcome(t *testing.T) {
	Convey("Given a Outcome server and client", t, func() {
		ctx := context.Background()
		server := strategy.NewOutcome(ctx)
		So(server, ShouldNotBeNil)

		client := strategy.Outcome_ServerToClient(server)
		So(client.IsValid(), ShouldBeTrue)

		Convey("When writing input values", func() {
			err := client.Write(ctx, func(params strategy.Outcome_write_Params) error {
				params.SetValue(100.0)
				params.SetAction(1)
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
