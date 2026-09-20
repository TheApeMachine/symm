package arithmetic_test

import (
	"context"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/arithmetic"
	"github.com/theapemachine/symm/nomagique/types"
)

func TestAddPrimitive(t *testing.T) {
	Convey("Given a native Add Cap'n Proto server", t, func() {
		server := arithmetic.NewAdd()
		So(server, ShouldNotBeNil)

		var (
			receivedVal  float64
			receivedDone bool
		)

		sink := types.NewFloat64Sink(
			func(ctx context.Context, val float64) error {
				receivedVal = val
				return nil
			},
			func(ctx context.Context) error {
				receivedDone = true
				return nil
			},
		)

		server.Downstream = sink

		client := arithmetic.Add_ServerToClient(server)
		So(client.IsValid(), ShouldBeTrue)

		Convey("When invoking Write with a and b", func() {
			ctx, _ := types.NextEvaluationContext(context.Background())

			err := client.Write(ctx, func(p arithmetic.Add_write_Params) error {
				p.SetA(2.5)
				p.SetB(3.5)
				return nil
			})
			So(err, ShouldBeNil)

			err = client.WaitStreaming()
			So(err, ShouldBeNil)
			_ = types.Float64Sink(sink).WaitStreaming()
			So(receivedVal, ShouldEqual, 6.0)

			Convey("When invoking Done, completion propagates downstream", func() {
				_, release := client.Done(ctx, nil)
				release()
				So(receivedDone, ShouldBeTrue)
			})
		})
	})
}
