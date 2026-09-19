package ui

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/pion/ice/v4"
	"github.com/pion/webrtc/v4"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/runtime"
	"github.com/theapemachine/symm/types"
)

func TestWebRTC(t *testing.T) {
	Convey("WebRTC negotiates a manifold data channel", t, func() {
		ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
		defer cancel()

		serverSettings := webrtc.SettingEngine{}
		serverSettings.SetIncludeLoopbackCandidate(true)
		serverSettings.SetICEMulticastDNSMode(ice.MulticastDNSModeDisabled)

		serverAPI := webrtc.NewAPI(webrtc.WithSettingEngine(serverSettings))
		serverConfig := &webrtc.Configuration{}

		hub := NewHub(ctx, nil, nil)
		hub.Transition(runtime.READY)
		rtc := NewWebRTC(ctx, hub, serverAPI, serverConfig)

		clientSettings := webrtc.SettingEngine{}
		clientSettings.SetIncludeLoopbackCandidate(true)
		clientSettings.SetICEMulticastDNSMode(ice.MulticastDNSModeDisabled)

		client, err := webrtc.NewAPI(
			webrtc.WithSettingEngine(clientSettings),
		).NewPeerConnection(webrtc.Configuration{})
		So(err, ShouldBeNil)
		defer client.Close()

		dataChannel, err := client.CreateDataChannel(types.ManifoldChannel, nil)
		So(err, ShouldBeNil)

		greeting := make(chan string, 1)
		dataChannel.OnMessage(func(message webrtc.DataChannelMessage) {
			greeting <- string(message.Data)
		})

		offer, err := client.CreateOffer(nil)
		So(err, ShouldBeNil)

		gathered := webrtc.GatheringCompletePromise(client)
		So(client.SetLocalDescription(offer), ShouldBeNil)

		select {
		case <-gathered:
		case <-ctx.Done():
			t.Fatal("ICE gathering timed out")
		}

		body, err := json.Marshal(client.LocalDescription())
		So(err, ShouldBeNil)

		req := httptest.NewRequest(
			http.MethodPost,
			"/webrtc/manifold",
			bytes.NewReader(body),
		)
		req.Header.Set("Content-Type", "application/json")

		resp, err := hub.app.Test(req, fiber.TestConfig{Timeout: 10 * time.Second})
		So(err, ShouldBeNil)
		defer resp.Body.Close()
		So(resp.StatusCode, ShouldEqual, http.StatusOK)

		var answer webrtc.SessionDescription
		So(json.NewDecoder(resp.Body).Decode(&answer), ShouldBeNil)
		So(client.SetRemoteDescription(answer), ShouldBeNil)

		select {
		case got := <-greeting:
			So(got, ShouldEqual, "Hello from Go server 👋")
		case <-ctx.Done():
			t.Fatal("data channel timed out")
		}

		So(rtc, ShouldNotBeNil)
	})

	Convey("WebRTC rejects malformed offers", t, func() {
		ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
		defer cancel()

		hub := NewHub(ctx, nil, nil)
		hub.Transition(runtime.READY)
		rtc := NewWebRTC(ctx, hub, nil, nil)

		req := httptest.NewRequest(
			http.MethodPost,
			"/webrtc/manifold",
			bytes.NewBufferString("not-json"),
		)
		req.Header.Set("Content-Type", "application/json")

		resp, err := hub.app.Test(req, fiber.TestConfig{Timeout: 10 * time.Second})
		So(err, ShouldBeNil)
		defer resp.Body.Close()
		So(resp.StatusCode, ShouldEqual, http.StatusBadRequest)
		So(rtc.Error(), ShouldNotBeNil)
	})
}
