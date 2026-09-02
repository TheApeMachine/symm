package websocket

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	gorillawebsocket "github.com/gorilla/websocket"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/runtime"
	"github.com/theapemachine/symm/types"
)

func TestFuturesLiveFail(t *testing.T) {
	Convey("Given the required futures session", t, func() {
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		futures := &FuturesLive{
			ctx:    ctx,
			cancel: cancel,
			status: runtime.NewStatus(),
		}
		first := errors.New("futures transport failed")
		second := errors.New("later failure")

		Convey("The first failure should persist and cancel the session", func() {
			futures.fail(first)
			futures.fail(second)

			So(errors.Is(futures.Error(), first), ShouldBeTrue)
			So(errors.Is(futures.Error(), second), ShouldBeFalse)
			So(futures.Status(), ShouldEqual, runtime.ERROR)
			So(futures.ctx.Err(), ShouldEqual, context.Canceled)
		})
	})
}

func TestFuturesLiveReconnect(t *testing.T) {
	Convey("Given a ready futures session with live subscriptions", t, func() {
		connections := make(chan *gorillawebsocket.Conn, 2)
		frames := make(chan struct {
			connection int64
			payload    []byte
		}, 16)
		serverErrors := make(chan error, 2)
		var accepted atomic.Int64
		upgrader := gorillawebsocket.Upgrader{
			CheckOrigin: func(*http.Request) bool { return true },
		}
		server := httptest.NewServer(http.HandlerFunc(func(
			writer http.ResponseWriter,
			request *http.Request,
		) {
			connection, err := upgrader.Upgrade(writer, request, nil)

			if err != nil {
				serverErrors <- err

				return
			}

			connectionID := accepted.Add(1)
			connections <- connection

			for {
				_, payload, err := connection.ReadMessage()

				if err != nil {
					return
				}

				frames <- struct {
					connection int64
					payload    []byte
				}{connection: connectionID, payload: payload}
			}
		}))
		defer server.Close()

		endpoint := "ws" + strings.TrimPrefix(server.URL, "http")
		futures := NewFuturesWithClient(
			t.Context(),
			endpoint,
			readyTestIngress("ticker", "trade"),
			nil,
		)
		defer futures.Close()

		futures.MarkReady()
		So(futures.SubFuturesTicker([]string{"PF_XBTUSD"}), ShouldBeNil)
		So(futures.SubFuturesTrades([]string{"PF_XBTUSD"}), ShouldBeNil)

		awaitFeeds := func(connectionID int64, expected ...string) bool {
			remaining := make(map[string]bool, len(expected))

			for _, feed := range expected {
				remaining[feed] = true
			}

			timeout := time.NewTimer(5 * time.Second)
			defer timeout.Stop()

			for len(remaining) > 0 {
				select {
				case frame := <-frames:
					if frame.connection != connectionID {
						continue
					}

					for feed := range remaining {
						if strings.Contains(string(frame.payload), `"feed":"`+feed+`"`) {
							delete(remaining, feed)
						}
					}
				case <-timeout.C:
					return false
				case <-serverErrors:
					return false
				}
			}

			return true
		}

		firstConnection := <-connections
		So(awaitFeeds(1, "heartbeat", "ticker", "trade"), ShouldBeTrue)

		Convey("An unexpected disconnect should restore the same feeds on a new connection", func() {
			So(firstConnection.Close(), ShouldBeNil)

			select {
			case <-connections:
			case <-time.After(5 * time.Second):
				So("reconnected", ShouldEqual, "timed out")
			}

			So(awaitFeeds(2, "heartbeat", "ticker", "trade"), ShouldBeTrue)
			So(futures.Error(), ShouldBeNil)
			So(futures.Status(), ShouldEqual, runtime.READY)
		})
	})
}

func TestFuturesFrameIdentity(t *testing.T) {
	Convey("Given a futures subscription acknowledgement", t, func() {
		raw := []byte(`{"event":"subscribed","feed":"ticker","product_ids":["PF_XBTUSD"]}`)

		Convey("Its lifecycle event should take precedence over the requested feed", func() {
			So(futuresFrameIdentity(raw), ShouldEqual, "subscribed")
		})
	})

	Convey("Given a futures market-data frame without a lifecycle event", t, func() {
		raw := []byte(`{"feed":"ticker","product_id":"PF_XBTUSD"}`)

		Convey("Its feed should remain the frame identity", func() {
			So(futuresFrameIdentity(raw), ShouldEqual, "ticker")
		})
	})
}

func TestFuturesLiveSubFuturesTicker(t *testing.T) {
	Convey("Given a connected futures session whose consumers are not ready", t, func() {
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		futures := &FuturesLive{
			ctx:    ctx,
			cancel: cancel,
			status: runtime.NewStatus().Transition(runtime.BUSY),
		}

		Convey("A ticker subscription should fail before writing to the socket", func() {
			err := futures.SubFuturesTicker([]string{"PF_XBTUSD"})

			So(err, ShouldNotBeNil)
			So(futures.Error(), ShouldNotBeNil)
			So(futures.Status(), ShouldEqual, runtime.ERROR)
		})
	})
}

func TestFuturesLiveSubFuturesTrades(t *testing.T) {
	Convey("Given a connected futures session whose consumers are not ready", t, func() {
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		futures := &FuturesLive{
			ctx:    ctx,
			cancel: cancel,
			status: runtime.NewStatus().Transition(runtime.BUSY),
		}

		Convey("A trade subscription should fail before writing to the socket", func() {
			err := futures.SubFuturesTrades([]string{"PF_XBTUSD"})

			So(err, ShouldNotBeNil)
			So(futures.Error(), ShouldNotBeNil)
			So(futures.Status(), ShouldEqual, runtime.ERROR)
		})
	})
}

func TestFuturesLiveSubFuturesBook(t *testing.T) {
	Convey("Given a connected futures session whose consumers are not ready", t, func() {
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		futures := &FuturesLive{
			ctx:    ctx,
			cancel: cancel,
			status: runtime.NewStatus().Transition(runtime.BUSY),
		}

		Convey("A book subscription should fail before writing to the socket", func() {
			err := futures.SubFuturesBook([]string{"PF_XBTUSD"})

			So(err, ShouldNotBeNil)
			So(futures.Error(), ShouldNotBeNil)
			So(futures.Status(), ShouldEqual, runtime.ERROR)
		})
	})
}

func TestFuturesLiveMarkReady(t *testing.T) {
	Convey("Given a connected futures session with a waiting consumer", t, func() {
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		futures := &FuturesLive{
			ctx:    ctx,
			cancel: cancel,
			status: runtime.NewStatus().Transition(runtime.BUSY),
			ingress: map[string]runtime.Ingress[*types.Envelope]{
				"ticker": &testIngress{
					status: runtime.NewStatus().Transition(runtime.WAITING),
				},
			},
		}

		Convey("Releasing the transport should fail the lifecycle", func() {
			futures.MarkReady()

			So(futures.Error(), ShouldNotBeNil)
			So(futures.Status(), ShouldEqual, runtime.ERROR)
		})
	})
}

func BenchmarkFuturesFrameIdentity(b *testing.B) {
	frames := map[string][]byte{
		"subscription_acknowledgement": []byte(
			`{"event":"subscribed","feed":"ticker","product_ids":["PF_XBTUSD"]}`,
		),
		"ticker": []byte(
			`{"feed":"ticker","product_id":"PF_XBTUSD"}`,
		),
	}

	for name, frame := range frames {
		b.Run(name, func(b *testing.B) {
			b.ReportAllocs()

			for iteration := 0; iteration < b.N; iteration++ {
				if futuresFrameIdentity(frame) == "" {
					b.Fatal("frame identity is empty")
				}
			}
		})
	}
}
