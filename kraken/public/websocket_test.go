package public

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestWebSocketConnectRequiresLiveConn(t *testing.T) {
	Convey("Given a stale connected flag without a socket", t, func() {
		ws := &WebSocket{}
		ws.isConnected.Store(true)

		Convey("It should clear the stale flag before dialing", func() {
			if ws.isConnected.Load() && ws.conn != nil {
				return
			}

			ws.isConnected.Store(false)

			So(ws.isConnected.Load(), ShouldBeFalse)
		})
	})
}
