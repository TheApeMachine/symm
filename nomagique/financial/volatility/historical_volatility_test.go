package volatility_test

import (
	"context"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/financial/volatility"
)

func TestHistoricalVolatility(t *testing.T) {
	Convey("Given a HistoricalVolatility server and client", t, func() {
		ctx := context.Background()
		server := volatility.NewHistoricalVolatility(ctx)
		So(server, ShouldNotBeNil)

		client := volatility.HistoricalVolatility_ServerToClient(server)
		So(client.IsValid(), ShouldBeTrue)

		Convey("When writing input values", func() {
			err := client.Write(ctx, func(params volatility.HistoricalVolatility_write_Params) error {
				params.SetPrice(100.0)
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
