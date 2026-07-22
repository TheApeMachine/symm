package ui

import (
	"context"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
)

func TestHubRetain(t *testing.T) {
	Convey("Given a hub draining Messages", t, func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		messages := make(chan []byte, 4)
		hub := NewHub(ctx, nil, nil, messages)
		defer hub.Close()

		Convey("When a balances frame is published before a client connects", func() {
			messages <- []byte(`{"balances":[{"asset":"USD","balance":1}]}`)

			Convey("It retains the latest balances payload", func() {
				deadline := time.Now().Add(time.Second)

				for time.Now().Before(deadline) {
					if value, ok := hub.subscribers.Load("balances"); ok {
						So(string(value.([]byte)), ShouldContainSubstring, `"balances"`)
						return
					}

					time.Sleep(5 * time.Millisecond)
				}

				So("retained", ShouldEqual, "missing")
			})
		})
	})
}
