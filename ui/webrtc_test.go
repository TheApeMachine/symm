package ui

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/pion/ice/v4"
	"github.com/pion/webrtc/v4"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/runtime"
	"github.com/theapemachine/symm/types"
)

/*
webrtcFixture encapsulates shared test setup and lifecycle for WebRTC tests,
ensuring constructor changes are localized to a single fixture owner.
*/
type webrtcFixture struct {
	ctx    context.Context
	cancel context.CancelFunc
	hub    *Hub
	rtc    *WebRTC
}

/*
withWebRTC decorates GoConvey tests with a managed WebRTC and Hub fixture,
following the GoConvey decorator and execution order conventions.
*/
func withWebRTC(t *testing.T, testFunc func(fx *webrtcFixture)) func() {
	return func() {
		ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
		hub := NewHub(ctx, nil, nil, nil, nil)
		hub.Transition(runtime.READY)

		rtc := NewWebRTC(ctx, hub)

		fx := &webrtcFixture{
			ctx:    ctx,
			cancel: cancel,
			hub:    hub,
			rtc:    rtc,
		}

		Reset(func() {
			cancel()
		})

		testFunc(fx)
	}
}

func (fx *webrtcFixture) newClient(dataChannelLabel string) (*webrtc.PeerConnection, *webrtc.DataChannel, chan string, chan struct{}) {
	settings := webrtc.SettingEngine{}
	settings.SetIncludeLoopbackCandidate(true)
	settings.SetICEMulticastDNSMode(ice.MulticastDNSModeDisabled)

	client, err := webrtc.NewAPI(
		webrtc.WithSettingEngine(settings),
	).NewPeerConnection(webrtc.Configuration{})
	So(err, ShouldBeNil)

	ordered := false
	retransmits := uint16(0)

	channel, err := client.CreateDataChannel(
		dataChannelLabel, &webrtc.DataChannelInit{
			Ordered:        &ordered,
			MaxRetransmits: &retransmits,
		},
	)
	So(err, ShouldBeNil)

	messages := make(chan string, 10)
	opened := make(chan struct{}, 1)

	channel.OnOpen(func() {
		opened <- struct{}{}
	})

	channel.OnMessage(func(msg webrtc.DataChannelMessage) {
		messages <- string(msg.Data)
	})

	return client, channel, messages, opened
}

func (fx *webrtcFixture) exchangeOffer(client *webrtc.PeerConnection) *http.Response {
	offer, err := client.CreateOffer(nil)
	So(err, ShouldBeNil)

	gathered := webrtc.GatheringCompletePromise(client)
	So(client.SetLocalDescription(offer), ShouldBeNil)

	select {
	case <-gathered:
	case <-fx.ctx.Done():
		So(fx.ctx.Err(), ShouldBeNil)
	}

	offerJSON, err := json.Marshal(client.LocalDescription())
	So(err, ShouldBeNil)

	req := httptest.NewRequest("POST", "/webrtc/manifold", bytes.NewReader(offerJSON))
	req.Header.Set("Content-Type", "application/json")

	resp, err := fx.hub.app.Test(req)
	So(err, ShouldBeNil)

	return resp
}

func TestWebRTC(t *testing.T) {
	Convey("Feature: WebRTC Transport", t, withWebRTC(t, func(fx *webrtcFixture) {
		Convey("Scenario: Client initiates peer connection with data channel", func() {
			client, channel, messages, opened := fx.newClient(types.ManifoldChannel)
			defer client.Close()

			Convey("When the client posts an SDP offer to /webrtc/manifold", func() {
				resp := fx.exchangeOffer(client)

				Convey("Then the server responds with 200 OK containing an SDP answer", func() {
					So(resp.StatusCode, ShouldEqual, 200)

					var answer webrtc.SessionDescription
					So(json.NewDecoder(resp.Body).Decode(&answer), ShouldBeNil)
					So(answer.Type, ShouldEqual, webrtc.SDPTypeAnswer)
					So(answer.SDP, ShouldNotBeEmpty)

					Convey("And when the client accepts the answer, the data channel opens", func() {
						So(client.SetRemoteDescription(answer), ShouldBeNil)

						select {
						case <-fx.ctx.Done():
							t.Fatal("Timeout waiting for data channel to open")
						case <-opened:
							So(channel.ReadyState(), ShouldEqual, webrtc.DataChannelStateOpen)
						}

						Convey("And the server sends its greeting to the client", func() {
							select {
							case <-fx.ctx.Done():
								t.Fatal("Timeout waiting for server greeting")
							case greeting := <-messages:
								So(greeting, ShouldEqual, "Hello from Go server 👋")
							}
						})

						Convey("And the client can send a message back to the server", func() {
							So(channel.SendText("Hello from Test Client"), ShouldBeNil)
						})
					})
				})
			})
		})

		Convey("Scenario: Client sends an invalid offer payload", func() {
			Convey("When the request body contains invalid JSON", func() {
				req := httptest.NewRequest("POST", "/webrtc/manifold", bytes.NewReader([]byte("not-json")))
				req.Header.Set("Content-Type", "application/json")

				resp, err := fx.hub.app.Test(req)
				So(err, ShouldBeNil)

				Convey("Then the server records the error and returns 400 Bad Request", func() {
					So(resp.StatusCode, ShouldEqual, 400)
					So(fx.rtc.Error(), ShouldNotBeNil)
				})
			})
		})

		Convey("Scenario: Trickle ICE candidate signaling", func() {
			Convey("When a client sends a valid candidate to /candidate", func() {
				candidateJSON, err := json.Marshal(webrtc.ICECandidateInit{
					Candidate: "candidate:1 1 UDP 2130706431 127.0.0.1 50000 typ host",
				})
				So(err, ShouldBeNil)

				req := httptest.NewRequest("POST", "/candidate", bytes.NewReader(candidateJSON))
				req.Header.Set("Content-Type", "application/json")

				resp, err := fx.hub.app.Test(req)
				So(err, ShouldBeNil)

				Convey("Then the server returns 200 OK", func() {
					So(resp.StatusCode, ShouldEqual, 200)
				})
			})

			Convey("When a client sends a malformed candidate", func() {
				req := httptest.NewRequest("POST", "/candidate", bytes.NewReader([]byte("bad-candidate")))
				req.Header.Set("Content-Type", "application/json")

				resp, err := fx.hub.app.Test(req)
				So(err, ShouldBeNil)

				Convey("Then the server records the error and returns 400 Bad Request", func() {
					So(resp.StatusCode, ShouldEqual, 400)
					So(fx.rtc.Error(), ShouldNotBeNil)
				})
			})
		})
	}))
}
