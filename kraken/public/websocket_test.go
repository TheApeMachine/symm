package public

import (
	"context"
	"net/http"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/spf13/viper"
	"github.com/theapemachine/qpool"
)

func TestNewWebSocketConnectMaxDelay(t *testing.T) {
	Convey("Given system.network.connection.max_delay", t, func() {
		viper.Set("system.network.connection.max_delay", 89)

		ctx := context.Background()
		pool := qpool.NewQ[any](ctx, 1, 2, nil)

		socket := NewWebSocket(ctx, pool)

		Convey("It should load the configured retry ceiling", func() {
			So(socket.connectMaxDelay, ShouldEqual, 89)
		})
	})
}

func TestDialHandshakeOK(t *testing.T) {
	Convey("Given a websocket upgrade response", t, func() {
		Convey("It should accept HTTP 101 Switching Protocols", func() {
			So(
				dialHandshakeOK(&http.Response{StatusCode: http.StatusSwitchingProtocols}, nil),
				ShouldBeTrue,
			)
		})

		Convey("It should reject HTTP 200 OK", func() {
			So(
				dialHandshakeOK(&http.Response{StatusCode: http.StatusOK}, nil),
				ShouldBeFalse,
			)
		})
	})
}
