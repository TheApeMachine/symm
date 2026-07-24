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
		hub := NewHub(ctx, nil, nil, messages, config.UIConfig{Addr: "127.0.0.1:0"})
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

		hub := NewHub(ctx, nil, nil, make(chan []byte, 1), config.UIConfig{Addr: "127.0.0.1:0"})
		defer hub.Close()

		Convey("When Publish retains replaceable keys", func() {
			hub.Publish([]byte(`{"measurements":[{"symbol":"BTC/USD"}],"causal":[{"symbol":"BTC/USD"}]}`))

			Convey("It caches streams omitted from the old key list", func() {
				So(len(hub.Cached("measurements")), ShouldBeGreaterThan, 0)
				So(len(hub.Cached("causal")), ShouldBeGreaterThan, 0)
				So(hub.generation.Load(), ShouldEqual, 1)
			})
		})
	})
}
