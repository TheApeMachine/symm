package ui

import (
	"context"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/config"
)

func TestHubRetain(t *testing.T) {
	Convey("Given a hub draining Messages", t, func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		messages := make(chan []byte, 4)
		hub := NewHub(ctx, nil, nil, nil, messages, make(chan []byte, 1), config.UIConfig{Addr: "127.0.0.1:0"})
		defer hub.Close()

		Convey("When a balances frame is published before a client connects", func() {
			messages <- []byte(`{"balances":[{"asset":"USD","balance":1}]}`)

			Convey("It retains the latest balances payload", func() {
				deadline := time.Now().Add(time.Second)

				for time.Now().Before(deadline) {
					if payload := hub.Cached("balances"); len(payload) > 0 {
						So(string(payload), ShouldContainSubstring, `"balances"`)
						return
					}

					time.Sleep(5 * time.Millisecond)
				}

				So("retained", ShouldEqual, "missing")
			})
		})
	})
}

func TestHubPublishGeneration(t *testing.T) {
	Convey("Given a hub", t, func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		hub := NewHub(ctx, nil, nil, nil, make(chan []byte, 1), make(chan []byte, 1), config.UIConfig{Addr: "127.0.0.1:0"})
		defer hub.Close()

		Convey("When Publish sees high-churn and durable keys together", func() {
			hub.Publish([]byte(`{"measurements":[{"symbol":"BTC/USD"}],"balances":[{"asset":"USD","balance":1}]}`))

			Convey("It retains only durable state", func() {
				So(len(hub.Cached("balances")), ShouldBeGreaterThan, 0)
				So(len(hub.Cached("measurements")), ShouldEqual, 0)
				So(hub.generation.Load(), ShouldEqual, 1)
			})
		})

		Convey("When Publish sees journal lifecycle state", func() {
			hub.Publish([]byte(`{"lifecycle":[{"symbol":"BTC/USD","state":"managing"}],"findings":[{"symbol":"BTC/USD"}]}`))

			Convey("It retains the journal rails for reconnect replay", func() {
				So(string(hub.Cached("lifecycle")), ShouldContainSubstring, `"managing"`)
				So(string(hub.Cached("findings")), ShouldContainSubstring, `"BTC/USD"`)
			})
		})
	})
}

/*
TestHubFanoutSaturationKeepsSessionAlive proves a full client queue drops
frames without cancelling the write loop — mute-without-close freezes the UI.
*/
func TestHubFanoutSaturationKeepsSessionAlive(t *testing.T) {
	Convey("Given a registered session whose outbound queue is already full", t, func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		hub := NewHub(ctx, nil, nil, nil, make(chan []byte, 1), make(chan []byte, 1), config.UIConfig{Addr: "127.0.0.1:0"})
		defer hub.Close()

		session := &clientSession{
			queue:  make(chan cachedFrame, 1),
			cancel: func() {},
			done:   make(chan struct{}),
		}
		session.queue <- cachedFrame{generation: 1, payload: []byte(`{"tick":1}`)}
		hub.clients.Store(session, struct{}{})

		before := hub.Dropped()
		hub.fanout(cachedFrame{generation: 2, payload: []byte(`{"tick":2}`)})

		Convey("It keeps the session registered and replaces the oldest frame", func() {
			So(hub.Dropped(), ShouldEqual, before)

			_, ok := hub.clients.Load(session)
			So(ok, ShouldBeTrue)
			So(len(session.queue), ShouldEqual, 1)
			So((<-session.queue).payload, ShouldResemble, []byte(`{"tick":2}`))
		})
	})
}
