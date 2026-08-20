package ui

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/fasthttp/websocket"
	"github.com/gofiber/fiber/v3"
	. "github.com/smartystreets/goconvey/convey"
)

const hubLifecycleTestTimeout = 5 * time.Second

func TestHubRun(t *testing.T) {
	Convey("Given a running dashboard hub", t, func() {
		ctx, cancel := context.WithCancel(context.Background())
		hub := NewHub(ctx, nil, nil, nil, nil, nil)
		hub.listenAddr = "127.0.0.1:0"
		listening := make(chan struct{})
		done := make(chan error, 1)
		hub.app.Hooks().OnListen(func(fiber.ListenData) error {
			close(listening)
			return nil
		})

		go func() { done <- hub.Run() }()

		select {
		case <-listening:
		case <-time.After(hubLifecycleTestTimeout):
			t.Fatal("hub did not start listening")
		}

		cancel()

		Convey("It should stop listening when its context is canceled", func() {
			select {
			case err := <-done:
				So(err, ShouldBeNil)
			case <-time.After(hubLifecycleTestTimeout):
				t.Fatal("hub did not stop after context cancellation")
			}
		})
	})
}

func TestExpectedDashboardWriteClosure(t *testing.T) {
	Convey("Given the close sentinel returned by the underlying connection", t, func() {
		err := fmt.Errorf("dashboard write: %w", websocket.ErrCloseSent)

		Convey("It should classify the completed close handshake as expected", func() {
			So(expectedDashboardWriteClosure(err), ShouldBeTrue)
		})
	})

	Convey("Given an unrelated write failure", t, func() {
		err := errors.New("unexpected write failure")

		Convey("It should preserve the failure for logging", func() {
			So(expectedDashboardWriteClosure(err), ShouldBeFalse)
		})
	})
}
