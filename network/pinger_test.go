package network

import (
	"context"
	"net/http"
	"net/http/httptest"
	goruntime "runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	gorilla "github.com/gorilla/websocket"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/nomagique/runtime"
)

func TestPingerStartStop(t *testing.T) {
	Convey("Start and Stop transition the pinger without panicking", t, func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		client := NewWebsocketClient(ctx)
		pinger := NewPinger(ctx, client)

		pinger.Start()
		So(pinger.Status(), ShouldEqual, runtime.READY)

		pinger.Stop()
		So(pinger.Status(), ShouldEqual, runtime.WAITING)
	})
}

func TestPingerWritePing(t *testing.T) {
	Convey("pinger loop emits websocket pings on the configured interval", t, func() {
		var pings atomic.Int32
		upgrader := gorilla.Upgrader{}

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			conn, err := upgrader.Upgrade(w, r, nil)
			if err != nil {
				return
			}

			conn.SetPingHandler(func(string) error {
				pings.Add(1)
				return conn.WriteControl(gorilla.PongMessage, nil, time.Now().Add(time.Second))
			})

			for {
				if _, _, err := conn.ReadMessage(); err != nil {
					return
				}
			}
		}))
		defer server.Close()

		wsURL := "ws" + strings.TrimPrefix(server.URL, "http")
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		client := NewWebsocketClient(ctx)
		So(client.Open(wsURL), ShouldBeNil)

		pinger := NewPinger(ctx, client)
		pinger.interval = 15 * time.Millisecond
		pinger.Start()

		deadline := time.Now().Add(500 * time.Millisecond)
		for time.Now().Before(deadline) && pings.Load() == 0 {
			goruntime.Gosched()
		}

		pinger.Stop()
		So(pings.Load(), ShouldBeGreaterThan, 0)
	})
}
