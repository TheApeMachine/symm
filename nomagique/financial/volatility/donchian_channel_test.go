package volatility_test

import (
	"context"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/financial/volatility"
)

func TestDonchianChannel(t *testing.T) {
	Convey("Given a DonchianChannel server and client", t, func() {
		ctx := context.Background()
		server := volatility.NewDonchianChannel(ctx)
		So(server, ShouldNotBeNil)

		client := volatility.DonchianChannel_ServerToClient(server)
		So(client.IsValid(), ShouldBeTrue)

		Convey("When writing input values", func() {
			err := client.Write(ctx, func(params volatility.DonchianChannel_write_Params) error {
				params.SetHigh(110.0)
				params.SetLow(90.0)
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
