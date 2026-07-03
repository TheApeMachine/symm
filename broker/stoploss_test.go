package broker

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/spf13/viper"
	"github.com/theapemachine/datura"
)

func TestStoplossUsesConfiguredTrailingOffset(t *testing.T) {
	Convey("Given a buy fill with a configured trailing stop offset", t, func() {
		oldOffset := viper.GetFloat64("trading.stop.trailing_offset_bps")
		viper.Set("trading.stop.trailing_offset_bps", 100.0)
		defer viper.Set("trading.stop.trailing_offset_bps", oldOffset)

		order := datura.Acquire("test", datura.APPJSON).
			WithScope("MATIC/USD").
			Poke(100.0, "last_price")
		defer order.Release()

		stoploss := NewStoploss(order, "MATIC/USD")

		Convey("It should persist the armed state on the order artifact", func() {
			So(stoploss, ShouldNotBeNil)
			So(stoploss.State, ShouldEqual, ARMED)
			So(datura.Peek[float64](order, "stoploss", "stop"), ShouldEqual, 99.0)
			So(datura.Peek[float64](order, "stoploss", "offset"), ShouldEqual, 0.01)
			So(datura.Peek[int](order, "stoploss", "state"), ShouldEqual, int(ARMED))
			So(datura.Peek[string](order, "stoploss", "state_label"), ShouldEqual, "ARMED")
		})

		Convey("It should ratchet upward and trigger when mark crosses the stop", func() {
			stoploss.Ratchet(105.0)
			So(datura.Peek[float64](order, "stoploss", "stop"), ShouldEqual, 103.95)
			So(stoploss.State, ShouldEqual, ARMED)

			stoploss.Ratchet(103.0)
			So(stoploss.State, ShouldEqual, TRIGGERED)
			So(datura.Peek[int](order, "stoploss", "state"), ShouldEqual, int(TRIGGERED))
			So(datura.Peek[string](order, "stoploss", "state_label"), ShouldEqual, "TRIGGERED")
			So(datura.Peek[float64](order, "stoploss", "trigger"), ShouldEqual, 103.0)
			So(datura.Peek[float64](order, "stoploss", "recent_marks", 0), ShouldEqual, 100.0)
			So(datura.Peek[float64](order, "stoploss", "recent_marks", 1), ShouldEqual, 105.0)
			So(datura.Peek[float64](order, "stoploss", "recent_marks", 2), ShouldEqual, 103.0)
		})
	})
}

func TestStoplossRatchetPreservesExitLifecycleState(t *testing.T) {
	Convey("Given a stoploss with an exit already submitted", t, func() {
		oldOffset := viper.GetFloat64("trading.stop.trailing_offset_bps")
		viper.Set("trading.stop.trailing_offset_bps", 100.0)
		defer viper.Set("trading.stop.trailing_offset_bps", oldOffset)

		order := datura.Acquire("test", datura.APPJSON).
			WithScope("MATIC/USD").
			Poke(100.0, "last_price")
		defer order.Release()

		stoploss := NewStoploss(order, "MATIC/USD")
		So(stoploss, ShouldNotBeNil)

		state := stoplossState(order)
		So(state, ShouldNotBeNil)
		state.setState(EXIT_SUBMITTED)
		state.ExitOrderID = "exit-1"
		state.RetryCount = 2
		writeStoplossState(order, state)
		stoploss.State = EXIT_SUBMITTED

		Convey("It should not create a second trigger while the exit is pending", func() {
			stoploss.Ratchet(98.0)
			state = stoplossState(order)

			So(stoploss.State, ShouldEqual, EXIT_SUBMITTED)
			So(state.State, ShouldEqual, int(EXIT_SUBMITTED))
			So(state.StateLabel, ShouldEqual, "EXIT_SUBMITTED")
			So(state.ExitOrderID, ShouldEqual, "exit-1")
			So(state.RetryCount, ShouldEqual, 2)
			So(state.Trigger, ShouldEqual, 0.0)
			So(datura.Peek[float64](order, "stoploss", "trigger"), ShouldEqual, 0.0)
		})
	})
}

func TestWriteStoplossStatePreservesInvalidAttributes(t *testing.T) {
	Convey("Given an order artifact with invalid attributes", t, func() {
		order := datura.Acquire("test", datura.APPJSON)
		defer order.Release()

		So(order.SetAttributes([]byte(`{"stoploss":`)), ShouldBeNil)

		Convey("It should leave the invalid bytes untouched", func() {
			writeStoplossState(order, &stoplossSnapshot{State: int(ARMED)})

			attributes, err := order.Attributes()
			So(err, ShouldBeNil)
			So(string(attributes), ShouldEqual, `{"stoploss":`)
		})
	})
}

func TestStoplossSnapshotPublishesTypedPayload(t *testing.T) {
	Convey("Given a populated stoploss snapshot", t, func() {
		state := newStoplossSnapshot(ARMED, 10.0, 0.02)
		state.NativeOrderID = "native-1"
		state.NativeExchangeOrderID = "exchange-1"
		state.NativeState = "working"
		state.ExitOrderID = "exit-1"
		state.RetryCount = 1

		payload := state.payload("MATIC/USD")

		Convey("It should expose the stoploss fields without map-state conversion", func() {
			So(payload["symbol"], ShouldEqual, "MATIC/USD")
			So(payload["state"], ShouldEqual, int(ARMED))
			So(payload["state_label"], ShouldEqual, "ARMED")
			So(payload["last_mark"], ShouldEqual, 10.0)
			So(payload["peak"], ShouldEqual, 10.0)
			So(payload["stop"], ShouldEqual, 9.8)
			So(payload["offset"], ShouldEqual, 0.02)
			So(payload["native_order_id"], ShouldEqual, "native-1")
			So(payload["native_exchange_order_id"], ShouldEqual, "exchange-1")
			So(payload["native_state"], ShouldEqual, "working")
			So(payload["exit_order_id"], ShouldEqual, "exit-1")
			So(payload["retry_count"], ShouldEqual, 1)
		})
	})
}
