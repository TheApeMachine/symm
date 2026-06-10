package ui

import (
	"context"
	"math"
	"net"
	"sync/atomic"
	"testing"

	"github.com/fasthttp/websocket"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/kraken/user"
	"github.com/valyala/fasthttp"
	"github.com/valyala/fasthttp/fasthttputil"
)

func TestHubRememberBalances(t *testing.T) {
	Convey("Given a balances ui frame", t, func() {
		hub := &Hub{}

		hub.rememberBalances(user.Balances{
			Asset: []user.Balance{{
				Asset:   "EUR",
				Balance: 250,
			}},
		})

		Convey("It should retain the latest snapshot for replay", func() {
			snapshot := hub.lastBalances.Load()

			So(snapshot, ShouldNotBeNil)
			So(len(snapshot.Asset), ShouldEqual, 1)
			So(snapshot.Asset[0].Balance, ShouldEqual, 250)
		})
	})
}

func TestFrontendClientSendRejectsNonFiniteJSON(t *testing.T) {
	Convey("Given a ui frame with non-finite floats", t, func() {
		listener := fasthttputil.NewInmemoryListener()
		var writeCalls atomic.Int32

		server := &fasthttp.Server{
			Handler: func(requestCtx *fasthttp.RequestCtx) {
				var upgrader websocket.FastHTTPUpgrader

				upgrader.Upgrade(requestCtx, func(conn *websocket.Conn) {
					for {
						if _, _, readErr := conn.ReadMessage(); readErr != nil {
							return
						}

						writeCalls.Add(1)
					}
				})
			},
		}

		go func() {
			_ = server.Serve(listener)
		}()

		t.Cleanup(func() {
			_ = server.Shutdown()
		})

		dialer := websocket.DefaultDialer
		dialer.NetDialContext = func(context.Context, string, string) (net.Conn, error) {
			return listener.Dial()
		}

		conn, _, dialErr := dialer.Dial("ws://symm.test/ws", nil)

		So(dialErr, ShouldBeNil)
		So(conn, ShouldNotBeNil)

		t.Cleanup(func() {
			_ = conn.Close()
		})

		client := &frontendClient{conn: conn}

		client.send(map[string]any{
			"type": "fluid",
			"re":   math.Inf(1),
		})

		Convey("It should fail JSON encoding before websocket write", func() {
			So(writeCalls.Load(), ShouldEqual, 0)
		})
	})
}
