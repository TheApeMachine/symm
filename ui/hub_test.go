package ui

import (
	"context"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/bytedance/sonic"
	wswebsocket "github.com/fasthttp/websocket"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/spf13/viper"
	"github.com/theapemachine/datura"
	"github.com/theapemachine/qpool"
)

func TestHubReceivesStateFrame(testingTB *testing.T) {
	Convey("Given the ui broadcast path used by trader and hub", testingTB, func() {
		ctx, cancel := context.WithCancel(context.Background())

		defer cancel()

		pool := qpool.NewQ[any](ctx, 1, 2, nil)
		subscription := pool.Subscribe("ui", nil)
		group := pool.CreateBroadcastGroup("ui")

		payload, err := sonic.Marshal(map[string]any{
			"type": "state",
			"measurements": []map[string]any{
				{
					"origin": "fluid",
					"scope":  "BTC/USD",
				},
			},
		})

		So(err, ShouldBeNil)

		artifact := datura.Acquire("trader", datura.Artifact_Type_json).
			WithPayload(payload)

		So(artifact, ShouldNotBeNil)
		artifact.WithDestination("ui")

		Convey("When trader publishes through the ui broadcast group", func() {
			So(group.Send(artifact), ShouldBeNil)

			received, waitErr := subscription.Wait(ctx)

			So(waitErr, ShouldBeNil)
			So(received, ShouldNotBeNil)

			wire := received.DecryptPayload()

			So(len(wire), ShouldBeGreaterThan, 0)

			var decoded map[string]any

			So(sonic.Unmarshal(wire, &decoded), ShouldBeNil)
			So(decoded["type"], ShouldEqual, "state")

			gaugeReadings, ok := decoded["measurements"].([]any)

			So(ok, ShouldBeTrue)
			So(len(gaugeReadings), ShouldEqual, 1)

			reading, ok := gaugeReadings[0].(map[string]any)

			So(ok, ShouldBeTrue)
			So(reading["origin"], ShouldEqual, "fluid")
			So(reading["scope"], ShouldEqual, "BTC/USD")
		})
	})
}

func TestNewHubCreatesRelay(testingTB *testing.T) {
	Convey("Given a new hub", testingTB, func() {
		ctx, cancel := context.WithCancel(context.Background())

		defer cancel()

		pool := qpool.NewQ[any](ctx, 1, 2, nil)
		hub := NewHub(ctx, pool)

		Convey("Then it has a UI subscription and app", func() {
			So(hub.uiSubscription, ShouldNotBeNil)
			So(hub.app, ShouldNotBeNil)
		})
	})
}

func TestHubWebSocketWritesPackedArtifact(testingTB *testing.T) {
	Convey("Given a hub websocket client", testingTB, func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		listener, err := net.Listen("tcp", "127.0.0.1:0")
		So(err, ShouldBeNil)

		listenAddr := listener.Addr().String()
		So(listener.Close(), ShouldBeNil)

		previousAddr := viper.GetString("ui.addr")
		viper.Set("ui.addr", listenAddr)
		defer viper.Set("ui.addr", previousAddr)

		pool := qpool.NewQ[any](ctx, 1, 2, nil)
		hub := NewHub(ctx, pool)
		serverErrors := make(chan error, 1)

		go func() {
			serverErrors <- hub.Run()
		}()

		defer func() {
			So(hub.Close(), ShouldBeNil)

			select {
			case err := <-serverErrors:
				if err != nil && !strings.Contains(err.Error(), "server is not running") {
					testingTB.Errorf("hub run failed: %v", err)
				}
			case <-time.After(time.Second):
				testingTB.Errorf("hub did not stop")
			}
		}()

		var conn *wswebsocket.Conn

		for deadline := time.Now().Add(2 * time.Second); time.Now().Before(deadline); {
			conn, _, err = wswebsocket.DefaultDialer.Dial("ws://"+listenAddr+"/ws", nil)

			if err == nil {
				break
			}

			time.Sleep(10 * time.Millisecond)
		}

		So(err, ShouldBeNil)
		defer conn.Close()

		payload := []byte(`{"type":"measurement","output":{"confidence":0.4}}`)
		artifact := datura.Acquire("pumpdump", datura.APPJSON).
			WithRole("measurement").
			WithScope("update").
			WithDestination("ui").
			WithPayload(payload)

		So(hub.uiBroadcast.Send(artifact), ShouldBeNil)

		messageType, wire, err := conn.ReadMessage()

		So(err, ShouldBeNil)
		So(messageType, ShouldEqual, wswebsocket.BinaryMessage)
		So(len(wire), ShouldBeGreaterThan, len(payload))

		var received datura.Artifact
		_, err = received.Unpack(wire)

		So(err, ShouldBeNil)
		So(string(received.DecryptPayload()), ShouldEqual, string(payload))
	})
}
