package calculus_test

import (
	"context"
	"math"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/calculus"
	"github.com/theapemachine/symm/nomagique/types"
)

func TestAtanhPrimitive(t *testing.T) {
	Convey("Given a native Atanh Cap'n Proto server", t, func() {
		server := calculus.NewAtanh()
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

		client := calculus.Atanh_ServerToClient(server)
		So(client.IsValid(), ShouldBeTrue)

		Convey("When invoking Write with a float64 value", func() {
			input := 0.5
			ctx, _ := types.NextEvaluationContext(context.Background())

			err := client.Write(ctx, func(p calculus.Atanh_write_Params) error {
				p.SetA(input)
				return nil
			})
			So(err, ShouldBeNil)

			err = client.WaitStreaming()
			So(err, ShouldBeNil)
			_ = types.Float64Sink(sink).WaitStreaming()

			expected := math.Atanh(input)
			So(receivedVal, ShouldEqual, expected)

			Convey("When invoking Done, completion propagates downstream", func() {
				_, release := client.Done(ctx, nil)
				release()
				So(receivedDone, ShouldBeTrue)
			})
		})
	})
}
