package websocket

import (
	"encoding/json"
	"testing"

	"github.com/krakenfx/api-go/v2/pkg/spot"
	. "github.com/smartystreets/goconvey/convey"
)

func TestAPIOnRoutesLevel3(t *testing.T) {
	Convey("Given an API with stub transports", t, func() {
		public := &stubConn{}
		private := &stubConn{}
		level3 := &stubConn{}
		api := NewAPI(public, private, level3)

		api.On("level3", func([]byte) {})
		api.On("ticker", func([]byte) {})
		api.On("balances", func([]byte) {})

		Convey("Then level3 callbacks register on the level3 transport", func() {
			So(len(level3.channels["level3"]), ShouldEqual, 1)
			So(len(public.channels["ticker"]), ShouldEqual, 1)
			So(len(private.channels["balances"]), ShouldEqual, 1)
		})
	})
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
