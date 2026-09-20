package hawkes_test

import (
	"context"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/statistic/hawkes"
)

func TestHawkesPipeline(t *testing.T) {
	ctx := context.Background()

	Convey("Given Hawkes primitives", t, func() {
		Convey("Assemble constructs time and mark from scalar args", func() {
			server := hawkes.NewAssemble()
			client := hawkes.Assemble_ServerToClient(server)
			So(client.IsValid(), ShouldBeTrue)

			err := client.Write(ctx, func(params hawkes.Assemble_write_Params) error {
				params.SetTimestamp(1000000000)
				params.SetSide("buy")
				params.SetSymbol("BTC/USD")
				return nil
			})
			So(err, ShouldBeNil)
			So(client.WaitStreaming(), ShouldBeNil)

			future, release := client.Done(ctx, nil)
			defer release()

			results, err := future.Struct()
			So(err, ShouldBeNil)
			So(results.Time(), ShouldEqual, 1000000000.0)
			So(results.Mark(), ShouldEqual, 1.0)
		})

		Convey("Process consumes time and mark and outputs statistics on Done", func() {
			server := hawkes.NewProcess()
			client := hawkes.Process_ServerToClient(server)
			So(client.IsValid(), ShouldBeTrue)

			now := time.Now().UnixNano()
			err := client.Write(ctx, func(params hawkes.Process_write_Params) error {
				params.SetTime(float64(now))
				params.SetMark(1.0)
				return nil
			})
			So(err, ShouldBeNil)
			So(client.WaitStreaming(), ShouldBeNil)

			future, release := client.Done(ctx, nil)
			defer release()

			results, err := future.Struct()
			So(err, ShouldBeNil)
			So(results.EventCount(), ShouldEqual, 1.0)
			So(results.BuyCount(), ShouldEqual, 1.0)
			So(results.SellCount(), ShouldEqual, 0.0)
		})
	})
}
