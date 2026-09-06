package ui

import (
	fastws "github.com/fasthttp/websocket"
	fiberws "github.com/gofiber/contrib/v3/websocket"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/types"
)

/*
TestHubWriteFrontend proves publication is safe before any dashboard client
connects. The guard must return without touching the nil connection — it
previously checked `hub.frontend != nil` and fell through to WriteMessage on
the nil connection, panicking on the first observe tick.

The hub no longer runs on the ring, so this is no longer about protecting the
pipeline from the encode; it is about the publisher goroutine surviving a run
with nobody watching. The encode itself must not happen at all in that case,
which the allocation count proves.
*/
func TestHubWriteFrontend(t *testing.T) {
	Convey("Given a hub with no dashboard clients", t, func() {
		hub := &Hub{}
		envelope := &types.Envelope{Key: "TEST/USD"}

		Convey("Writing returns without panicking", func() {
			So(func() { hub.writeFrontend(envelope) }, ShouldNotPanic)
		})

		Convey("Writing does not allocate a discarded FlatBuffer snapshot", func() {
			allocations := testing.AllocsPerRun(100, func() {
				hub.writeFrontend(envelope)
			})

			So(allocations, ShouldEqual, 0)
		})
	})
	Convey("A failed browser write detaches the dead connection immediately", t, func() {
		accepted := make(chan *fastws.Conn, 1)
		upgrader := fastws.Upgrader{}
		server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
			connection, err := upgrader.Upgrade(response, request, nil)
			if err != nil {
				t.Error(err)
				return
			}
			accepted <- connection
		}))
		defer server.Close()
		client, response, err := fastws.DefaultDialer.Dial("ws"+strings.TrimPrefix(server.URL, "http"), nil)
		So(err, ShouldBeNil)
		if response.Body != nil {
			defer response.Body.Close()
		}
		defer client.Close()
		connection := <-accepted
		So(connection.Close(), ShouldBeNil)
		hub := &Hub{frontend: &fiberws.Conn{Conn: connection}}
		hub.writeFrontend(&types.Envelope{Key: "TEST/USD"})
		So(hub.frontend, ShouldBeNil)
		So(testing.AllocsPerRun(10, func() { hub.writeFrontend(&types.Envelope{Key: "TEST/USD"}) }), ShouldEqual, 0)
	})

}
