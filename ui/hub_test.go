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
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data/sequence"
	"github.com/theapemachine/symm/nomagique/geometry"
	"github.com/theapemachine/symm/nomagique/runtime"
	"github.com/theapemachine/symm/nomagique/store"
	"github.com/theapemachine/symm/signal"
	wire "github.com/theapemachine/symm/telemetry/generated/telemetry"
)

func newHubConnection(testingContext testing.TB) (*Hub, *websocket.Conn) {
	testingContext.Helper()
	ctx, cancel := context.WithCancel(testingContext.Context())
	hub := NewHub(ctx, nil, nil, nil, nil)
	testingContext.Cleanup(func() {
		cancel()

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
		So(len(frame.Rows), ShouldEqual, 1)

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
	})
}

func TestHubRegisterSignals(t *testing.T) {
	Convey("Hub with registered signals encodes actual signal metrics into frame", t, func() {
		ctx := context.Background()
		grid := store.NewGrid[*geometry.Coordinate]()
		signals, err := signal.LoadAll(ctx, grid)
		So(err, ShouldBeNil)
		So(len(signals), ShouldEqual, 15)

		hub := NewHub(ctx, nil, nil, nil, nil)
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

		ptr, ok := hub.queue.Dequeue()
		So(ok, ShouldBeTrue)
		So(ptr, ShouldNotBeNil)

		payload := *(*[]byte)(ptr)
		frame := wire.GetRootAsMeasurementsFrame(payload, 0).UnPack()
		So(frame, ShouldNotBeNil)
		So(len(frame.Rows), ShouldEqual, 16)
		So(frame.Rows[0].Source, ShouldEqual, "training")

		sourceMap := make(map[string]bool)
		for _, r := range frame.Rows[1:] {
			sourceMap[r.Source] = true
		}
		So(sourceMap["correlation:ticker"], ShouldBeTrue)
		So(sourceMap["cvd:trade"], ShouldBeTrue)
		So(sourceMap["depthflow:level3"], ShouldBeTrue)
		So(sourceMap["derivatives:ticker"], ShouldBeTrue)
		So(sourceMap["derivatives:trade"], ShouldBeTrue)
		So(sourceMap["hawkes:trade"], ShouldBeTrue)
		So(sourceMap["leadlag:ticker"], ShouldBeTrue)
		So(sourceMap["liquidity:ticker"], ShouldBeTrue)
		So(sourceMap["morphology:level3"], ShouldBeTrue)
		So(sourceMap["pumpdump:level3"], ShouldBeTrue)
		So(sourceMap["pumpdump:ticker"], ShouldBeTrue)
		So(sourceMap["pumpdump:trade"], ShouldBeTrue)
		So(sourceMap["sentiment:ticker"], ShouldBeTrue)
		So(sourceMap["toxicity:level3"], ShouldBeTrue)
		So(sourceMap["toxicity:trade"], ShouldBeTrue)

		Convey("Incoming market data updates signal metrics streamed to hub", func() {
			writeQuery := core.NewQuery[*geometry.Coordinate, core.Connectable[*geometry.Coordinate]](nil, core.Write)
			marketPayload := map[string]any{
				"ticker": map[string]any{
					"data": map[string]any{
						"symbol": "BTC/USD",
						"last":   65000.0,
					},
				},
			}

			for range grid.Next(writeQuery.Next(sequence.NewValue(marketPayload))) {
			}

			for range hub.Next(sequence.NewValue(*eval)) {
			}

			ptr2, ok2 := hub.queue.Dequeue()
			So(ok2, ShouldBeTrue)
			frame2 := wire.GetRootAsMeasurementsFrame(*(*[]byte)(ptr2), 0).UnPack()
			So(frame2, ShouldNotBeNil)
			So(len(frame2.Rows), ShouldEqual, 16)
		})
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
