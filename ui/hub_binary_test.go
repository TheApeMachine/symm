package ui

import (
	"context"
	"net"
	"testing"
	"time"

	flatbuffers "github.com/google/flatbuffers/go"
	"github.com/gorilla/websocket"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/transport"
	"github.com/theapemachine/symm/telemetry"
	wire "github.com/theapemachine/symm/telemetry/generated/telemetry"
	"github.com/theapemachine/symm/types"
)

func TestHubForecastPublication(t *testing.T) {
	Convey("Given a dashboard client subscribed before a typed forecast is published", t, func() {
		listener, err := net.Listen("tcp", "127.0.0.1:0")
		So(err, ShouldBeNil)
		thesis := types.NewThesis(context.Background(), transport.NewMapReduce[*types.UIFrame](nil, nil, nil))
		hub := NewHub(context.Background(), thesis, nil, nil, nil, transport.NewMapReduce[types.FluidFrame](nil, nil, nil))
		serveErr := make(chan error, 1)

		go func() {
			serveErr <- hub.app.Listener(listener)
		}()

		Reset(func() {
			hub.cancel()
			So(hub.Close(), ShouldBeNil)

			select {
			case err = <-serveErr:
				So(err, ShouldBeNil)
			case <-time.After(time.Second):
				t.Fail()
			}
		})

		conn, _, err := websocket.DefaultDialer.Dial(
			"ws://"+listener.Addr().String()+"/ws",
			nil,
		)
		So(err, ShouldBeNil)
		Reset(func() { So(conn.Close(), ShouldBeNil) })
		peer, _, err := websocket.DefaultDialer.Dial(
			"ws://"+listener.Addr().String()+"/ws",
			nil,
		)
		So(err, ShouldBeNil)
		Reset(func() { So(peer.Close(), ShouldBeNil) })

		thesis.UI().Push(&wire.FrameT{
			Type: wire.FrameResonanceFrame,
			Value: &wire.ResonanceFrameT{
				Rows: []*wire.ResonanceT{{
					Source: string(types.SourceResonance), Symbol: "BTC/USD",
					Forecast: &wire.ResonanceForecastT{
						Aggregate: &wire.PosteriorT{
							Value: 0.02, Scale: 0.01, DegreesOfFreedom: 2, Ready: true,
						},
					},
				}},
			},
		})
		So(conn.SetReadDeadline(time.Now().Add(time.Second)), ShouldBeNil)

		messageType, received, err := conn.ReadMessage()
		So(err, ShouldBeNil)
		So(messageType, ShouldEqual, websocket.BinaryMessage)
		So(peer.SetReadDeadline(time.Now().Add(time.Second)), ShouldBeNil)
		_, peerReceived, err := peer.ReadMessage()
		So(err, ShouldBeNil)
		So(flatbuffers.BufferHasIdentifier(received, telemetry.BatchIdentifier), ShouldBeTrue)
		So(flatbuffers.BufferHasIdentifier(peerReceived, telemetry.BatchIdentifier), ShouldBeTrue)
		batch := wire.GetRootAsBatch(received, 0).UnPack()
		peerBatch := wire.GetRootAsBatch(peerReceived, 0).UnPack()
		So(batch.Frames, ShouldHaveLength, 1)
		So(peerBatch.Frames, ShouldHaveLength, 1)
		So(batch.Frames[0].Frame.Type, ShouldEqual, wire.FrameResonanceFrame)
		So(peerBatch.Frames[0].Frame.Type, ShouldEqual, wire.FrameResonanceFrame)
		frame := batch.Frames[0].Frame.Value.(*wire.ResonanceFrameT)

		Convey("It should receive the explicit supported-horizon forecast", func() {
			So(frame.Rows, ShouldHaveLength, 1)
			So(frame.Rows[0].Source, ShouldEqual, string(types.SourceResonance))
			So(frame.Rows[0].Symbol, ShouldEqual, "BTC/USD")
			So(frame.Rows[0].Forecast.Aggregate.Ready, ShouldBeTrue)
			So(frame.Rows[0].Forecast.Aggregate.Value, ShouldEqual, 0.02)
		})
	})
}

func TestHubReadinessPublication(t *testing.T) {
	Convey("Given a dashboard client subscribed before readiness changes", t, func() {
		listener, err := net.Listen("tcp", "127.0.0.1:0")
		So(err, ShouldBeNil)
		thesis := types.NewThesis(context.Background(), transport.NewMapReduce[*types.UIFrame](nil, nil, nil))
		hub := NewHub(context.Background(), thesis, nil, nil, nil, transport.NewMapReduce[types.FluidFrame](nil, nil, nil))
		serveErr := make(chan error, 1)

		go func() {
			serveErr <- hub.app.Listener(listener)
		}()

		Reset(func() {
			hub.cancel()
			So(hub.Close(), ShouldBeNil)

			select {
			case err = <-serveErr:
				So(err, ShouldBeNil)
			case <-time.After(time.Second):
				t.Fail()
			}
		})

		conn, _, err := websocket.DefaultDialer.Dial(
			"ws://"+listener.Addr().String()+"/ws",
			nil,
		)
		So(err, ShouldBeNil)
		Reset(func() { So(conn.Close(), ShouldBeNil) })

		So(conn.SetReadDeadline(time.Now().Add(time.Second)), ShouldBeNil)
	})
}
