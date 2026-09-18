package ui

import (
	"context"
	"net"
	"testing"
	"time"
	"unsafe"

	"github.com/gofiber/fiber/v3"
	"github.com/gorilla/websocket"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/cognition"
	"github.com/theapemachine/symm/nomagique/data/sequence"
	"github.com/theapemachine/symm/nomagique/runtime"
	wire "github.com/theapemachine/symm/telemetry/generated/telemetry"
)

func newHubConnection(testingContext testing.TB) (*Hub, *websocket.Conn) {
	testingContext.Helper()
	ctx, cancel := context.WithCancel(testingContext.Context())
	hub := NewHub(ctx, nil, nil)
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

	return hub, connection
}

func TestNewHub(t *testing.T) {
	Convey("A frontend connects while application startup is incomplete", t, func() {
		hub, connection := newHubConnection(t)
		// Hub has no READY status yet, so the websocket loop idles.
		time.Sleep(30 * time.Millisecond)

		Convey("Once activated, the same connection receives successive frames", func() {
			hub.Transition(runtime.READY)

			for _, payload := range [][]byte{{1, 2, 3}, {4, 5, 6}} {
				hub.queue.Enqueue(unsafe.Pointer(&payload))
				So(connection.SetReadDeadline(time.Now().Add(time.Second)), ShouldBeNil)
				messageType, received, err := connection.ReadMessage()
				So(err, ShouldBeNil)
				So(messageType, ShouldEqual, websocket.BinaryMessage)
				So(received, ShouldResemble, payload)
			}
		})
	})
}

func TestHubNextEvaluation(t *testing.T) {
	Convey("Hub.Next encodes Evaluation into FlatBuffers MeasurementsFrame for WebSocket", t, func() {
		hub, connection := newHubConnection(t)
		hub.Transition(runtime.READY)

		eval := &cognition.Evaluation{
			Step:       42,
			Surprisal:  1.23,
			Ambiguity:  0.45,
			Confidence: 0.88,
			Contrast:   0.67,
			Support:    100,
		}

		for range hub.Next(sequence.NewValue(*eval)) {
		}

		So(connection.SetReadDeadline(time.Now().Add(time.Second)), ShouldBeNil)
		messageType, received, err := connection.ReadMessage()
		So(err, ShouldBeNil)
		So(messageType, ShouldEqual, websocket.BinaryMessage)

		frame := wire.GetRootAsMeasurementsFrame(received, 0).UnPack()
		So(frame, ShouldNotBeNil)
		So(len(frame.Rows), ShouldEqual, 12)

		row := frame.Rows[0]
		So(row.Source, ShouldEqual, "training")
		So(row.Symbol, ShouldEqual, "BTC/USD")
		So(row.Tick, ShouldEqual, 42)

		metricMap := make(map[string]float64)
		for _, m := range row.Metrics {
			metricMap[m.Name] = m.Raw
		}

		So(metricMap["surprisal"], ShouldEqual, 1.23)
		So(metricMap["ambiguity"], ShouldEqual, 0.45)
		So(metricMap["confidence"], ShouldEqual, 0.88)
		So(metricMap["contrast"], ShouldEqual, 0.67)
		So(metricMap["support"], ShouldEqual, 100)
		So(metricMap["steps"], ShouldEqual, 42)

		kernelMap := make(map[string]bool)
		for _, r := range frame.Rows[1:] {
			kernelMap[r.Source] = true
			So(r.Snr, ShouldEqual, 0.88)
		}
		So(kernelMap["cvd"], ShouldBeTrue)
		So(kernelMap["depthflow"], ShouldBeTrue)
		So(kernelMap["correlation"], ShouldBeTrue)
	})
}

func BenchmarkNewHub(b *testing.B) {
	hub, connection := newHubConnection(b)
	hub.Transition(runtime.READY)
	payload := []byte("dashboard frame")
	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		hub.queue.Enqueue(unsafe.Pointer(&payload))

		if _, _, err := connection.ReadMessage(); err != nil {
			b.Fatal(err)
		}
	}
}
