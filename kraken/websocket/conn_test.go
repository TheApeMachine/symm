package websocket

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/krakenfx/api-go/v2/pkg/spot"
	"github.com/spf13/viper"
	. "github.com/smartystreets/goconvey/convey"
)

func TestAPIOnRoutesLevel3(t *testing.T) {
	Convey("Given a live API with stub transports", t, func() {
		viper.Set("trading.model", "live")

		public := &stubConn{}
		private := &stubConn{}
		api := NewAPI(public, private, nil)

		api.On("level3", func([]byte) {})
		api.On("ticker", func([]byte) {})
		api.On("balances", func([]byte) {})

		Convey("Then level3 and private-channel callbacks register on the private transport", func() {
			So(len(private.channels["level3"]), ShouldEqual, 1)
			So(len(public.channels["ticker"]), ShouldEqual, 1)
			So(len(private.channels["balances"]), ShouldEqual, 1)
		})
	})

	Convey("Given a paper API with stub transports", t, func() {
		viper.Set("trading.model", "paper")

		public := &stubConn{}
		private := &stubConn{}
		paper := NewPaper(context.Background(), newTestSimulator())
		api := NewAPI(public, private, paper)

		api.On("level3", func([]byte) {})
		api.On("ticker", func([]byte) {})
		api.On("balances", func([]byte) {})
		api.On("executions", func([]byte) {})
		api.On("add_order", func([]byte) {})

		Convey("Then level3 still registers on the private transport", func() {
			So(len(private.channels["level3"]), ShouldEqual, 1)
			So(len(public.channels["ticker"]), ShouldEqual, 1)
		})

		Convey("Then balances, executions, and add_order register on the paper transport instead", func() {
			So(len(private.channels["balances"]), ShouldEqual, 0)

			_, ok := paper.sync.Load("balances")
			So(ok, ShouldBeTrue)

			_, ok = paper.sync.Load("executions")
			So(ok, ShouldBeTrue)

			_, ok = paper.sync.Load("add_order")
			So(ok, ShouldBeTrue)
		})
	})
}

func newTestSimulator() *Simulator {
	simulator := NewSimulator()
	simulator.Initialize()
	return simulator
}

type stubConn struct {
	channels map[string][]func([]byte)
}

func (stub *stubConn) Client() *spot.WebSocket { return nil }

func (stub *stubConn) On(channel string, action func([]byte)) {
	if stub.channels == nil {
		stub.channels = map[string][]func([]byte){}
	}

	stub.channels[channel] = append(stub.channels[channel], action)
}

func (stub *stubConn) Write(params json.Marshaler) error { return nil }

func (stub *stubConn) Close() {}

func (stub *stubConn) Post(path string, params json.Marshaler) ([]byte, error) {
	return nil, nil
}
