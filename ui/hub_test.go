package ui

import (
	"context"
	"net"
	"sync/atomic"
	"testing"
	"time"
	"unsafe"

	"github.com/gofiber/fiber/v3"
	"github.com/gorilla/websocket"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/runtime"
)

// hubTestTee observes reads at the existing runtime.Tee boundary.
// Payloads are opaque to Hub; encoding remains UITee's responsibility.
type hubTestTee struct {
	reads  atomic.Int64
	frames chan []byte
}

func (tee *hubTestTee) Push(*data.Measurement[float64]) {}
func (tee *hubTestTee) Close() error                    { return nil }

func (tee *hubTestTee) Next() unsafe.Pointer {
	tee.reads.Add(1)

	select {
	case frame := <-tee.frames:
		return unsafe.Pointer(&frame)
	default:
		return nil
	}
}

func newHubConnection(testingContext testing.TB) (*Hub, *hubTestTee, *websocket.Conn) {
	testingContext.Helper()
	ctx, cancel := context.WithCancel(testingContext.Context())
	tee := &hubTestTee{frames: make(chan []byte, 1)}
	hub := NewHub(ctx, nil, nil, tee)
	testingContext.Cleanup(func() {
		cancel()

		if err := hub.Fluid.Close(); err != nil {
			testingContext.Error(err)
		}

		if err := hub.Close(); err != nil {
			testingContext.Error(err)
		}
	})
	listener, err := net.Listen("tcp", "127.0.0.1:0")

	if err != nil {
		testingContext.Fatal(err)
	}

	finished := make(chan error, 1)

	go func() {
		finished <- hub.app.Listener(listener, fiber.ListenConfig{DisableStartupMessage: true})
	}()

	testingContext.Cleanup(func() {
		cancel()

		if err := hub.app.ShutdownWithTimeout(time.Second); err != nil {
			testingContext.Error(err)
		}

		if err := <-finished; err != nil {
			testingContext.Error(err)
		}
	})
	connection, response, err := websocket.DefaultDialer.DialContext(
		ctx, "ws://"+listener.Addr().String()+"/ws", nil,
	)

	if response != nil {
		if err := response.Body.Close(); err != nil {
			testingContext.Error(err)
		}
	}

	if err != nil {
		testingContext.Fatal(err)
	}

	testingContext.Cleanup(func() {
		if err := connection.Close(); err != nil {
			testingContext.Error(err)
		}
	})

	return hub, tee, connection
}

func TestNewHub(t *testing.T) {
	Convey("A frontend connects while application startup is incomplete", t, func() {
		hub, tee, connection := newHubConnection(t)
		// Observe several existing 10ms idle polling cycles before activation.
		time.Sleep(30 * time.Millisecond)
		So(tee.reads.Load(), ShouldEqual, 0)

		Convey("Once activated, the same connection receives successive frames", func() {
			hub.Transition(runtime.READY)

			for _, payload := range [][]byte{{1, 2, 3}, {4, 5, 6}} {
				tee.frames <- payload
				So(connection.SetReadDeadline(time.Now().Add(time.Second)), ShouldBeNil)
				messageType, received, err := connection.ReadMessage()
				So(err, ShouldBeNil)
				So(messageType, ShouldEqual, websocket.BinaryMessage)
				So(received, ShouldResemble, payload)
			}
		})
	})
}

func BenchmarkNewHub(b *testing.B) {
	hub, tee, connection := newHubConnection(b)
	hub.Transition(runtime.READY)
	payload := []byte("dashboard frame")
	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		tee.frames <- payload

		if _, _, err := connection.ReadMessage(); err != nil {
			b.Fatal(err)
		}
	}
}
