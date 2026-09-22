package momentum_test

import (
	"context"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/financial/momentum"
)

func TestChaikinOscillator(t *testing.T) {
	Convey("Given a ChaikinOscillator server and client", t, func() {
		ctx := context.Background()
		server := momentum.NewChaikinOscillator(ctx)
		So(server, ShouldNotBeNil)

		client := momentum.ChaikinOscillator_ServerToClient(server)
		So(client.IsValid(), ShouldBeTrue)

		Convey("When writing input values", func() {
			err := client.Write(ctx, func(params momentum.ChaikinOscillator_write_Params) error {
				params.SetHigh(110.0)
				params.SetLow(90.0)
				params.SetClose(100.0)
				params.SetVolume(1000.0)
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
