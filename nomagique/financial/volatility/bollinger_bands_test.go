package volatility_test

import (
	"context"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/financial/volatility"
)

func TestBollingerBands(t *testing.T) {
	Convey("Given a BollingerBands server and client", t, func() {
		ctx := context.Background()
		server := volatility.NewBollingerBands(ctx)
		So(server, ShouldNotBeNil)

		client := volatility.BollingerBands_ServerToClient(server)
		So(client.IsValid(), ShouldBeTrue)

		Convey("When writing input values", func() {
			err := client.Write(ctx, func(params volatility.BollingerBands_write_Params) error {
				params.SetClose(100.0)
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
