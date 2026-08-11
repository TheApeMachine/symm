package ui

import (
	"context"
	"encoding/json"
	"net"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/datura"
	"github.com/theapemachine/symm/types"
	"github.com/theapemachine/symm/utils"
)

func TestHubForecastPublication(t *testing.T) {
	Convey("Given a dashboard client subscribed before a typed forecast is published", t, func() {
		listener, err := net.Listen("tcp", "127.0.0.1:0")
		So(err, ShouldBeNil)
		messages := make(chan []byte, 1)
		hub := NewHub(context.Background(), nil, nil, nil, messages, make(chan types.FluidFrame, 1))
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

		forecast, err := types.NewResonanceForecast(
			[]float64{0.01, 0.02}, []float64{1, 0.5}, 2, 0.75,
		)
		So(err, ShouldBeNil)
		reading := types.ResonanceReading{
			Source:   types.SourceResonance,
			Symbol:   "BTC/USD",
			Forecast: forecast,
		}

		utils.Publish(messages, datura.NewMap("resonance", []any{reading}))
		So(conn.SetReadDeadline(time.Now().Add(time.Second)), ShouldBeNil)

		messageType, received, err := conn.ReadMessage()
		So(err, ShouldBeNil)
		So(messageType, ShouldEqual, websocket.TextMessage)
		So(peer.SetReadDeadline(time.Now().Add(time.Second)), ShouldBeNil)
		_, peerReceived, err := peer.ReadMessage()
		So(err, ShouldBeNil)
		So(peerReceived, ShouldResemble, received)

		var frame struct {
			Resonance []types.ResonanceReading `json:"resonance"`
		}
		So(json.Unmarshal(received, &frame), ShouldBeNil)

		Convey("It should receive the explicit supported-horizon forecast", func() {
			So(frame.Resonance, ShouldHaveLength, 1)
			So(frame.Resonance[0].Forecast, ShouldNotBeNil)
			So(frame.Resonance[0].Forecast.Validate(), ShouldBeNil)
			So(frame.Resonance[0].Forecast.SupportedHorizon, ShouldEqual, 2)
			So(frame.Resonance[0].Forecast.ExpectedReturn,
				ShouldEqual, forecast.ExpectedReturn)
		})
	})
}

func TestHubReadinessPublication(t *testing.T) {
	Convey("Given a dashboard client subscribed before readiness changes", t, func() {
		listener, err := net.Listen("tcp", "127.0.0.1:0")
		So(err, ShouldBeNil)
		messages := make(chan []byte, 1)
		hub := NewHub(context.Background(), nil, nil, nil, messages, make(chan types.FluidFrame, 1))
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
