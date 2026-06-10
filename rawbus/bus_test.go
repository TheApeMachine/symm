package rawbus

import (
	"context"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/internal"
	"github.com/theapemachine/symm/kraken/trading"
	"github.com/theapemachine/symm/logic"
)

func TestDecodeAction(t *testing.T) {
	Convey("Given a raw order frame", t, func() {
		action := &logic.Action{
			Type:   logic.ActionMarket,
			Side:   trading.Buy,
			Symbol: "BTC/USD",
		}

		row := &qpool.QValue[any]{
			Type:  TypeOrder.String(),
			Value: action,
		}

		Convey("It should decode the action payload", func() {
			decoded, err := DecodeAction(row)

			So(err, ShouldBeNil)
			So(decoded, ShouldEqual, action)
		})
	})
}

func TestSend(t *testing.T) {
	Convey("Given a raw bus publisher", t, func() {
		ctx := context.Background()
		pool := qpool.NewQ[any](ctx, 2, 4, nil)

		bus := internal.NewBus(
			ctx,
			pool,
			[]internal.Channel{internal.ChannelRaw},
			[]internal.Subscription{
				internal.Subscribe(internal.ChannelRaw, "raw:test"),
			},
		)

		action := &logic.Action{Symbol: "BTC/USD"}

		So(Send(bus, TypeActions, action), ShouldBeNil)

		row, err := bus.Receive(internal.ChannelRaw)

		So(err, ShouldBeNil)
		So(TypeOf(row), ShouldEqual, TypeActions)
	})
}

func BenchmarkDecodeAction(b *testing.B) {
	row := &qpool.QValue[any]{
		Type: TypeOrder.String(),
		Value: &logic.Action{
			Symbol: "BTC/USD",
		},
	}

	b.ReportAllocs()

	for b.Loop() {
		_, _ = DecodeAction(row)
	}
}
