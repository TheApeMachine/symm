package ui

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/pion/webrtc/v4"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/runtime"
	"github.com/theapemachine/symm/types"
)

func TestWebRTC(t *testing.T) {
	Convey("Given a WebRTC service on the Hub", t, func() {
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()

		hub := NewHub(ctx, nil, nil, nil)
		So(hub.WebRTC, ShouldNotBeNil)

		Convey("Answering a client offer produces a valid SDP answer", func() {
			client, err := webrtc.NewPeerConnection(webrtc.Configuration{})
			So(err, ShouldBeNil)
			defer func() { So(client.Close(), ShouldBeNil) }()

			ordered := false
			retransmits := uint16(0)
			_, err = client.CreateDataChannel(types.ManifoldChannel, &webrtc.DataChannelInit{
				Ordered:        &ordered,
				MaxRetransmits: &retransmits,
			})
			So(err, ShouldBeNil)

			offer, err := client.CreateOffer(nil)
			So(err, ShouldBeNil)
			So(client.SetLocalDescription(offer), ShouldBeNil)

			offerBytes, err := json.Marshal(offer)
			So(err, ShouldBeNil)

			req := httptest.NewRequest("POST", "/offer", bytes.NewReader(offerBytes))
			req.Header.Set("Content-Type", "application/json")
			res, err := hub.app.Test(req)
			So(err, ShouldBeNil)
			So(res.StatusCode, ShouldEqual, 200)

			body, err := io.ReadAll(res.Body)
			So(err, ShouldBeNil)

			var answer webrtc.SessionDescription
			So(json.Unmarshal(body, &answer), ShouldBeNil)
			So(answer.Type, ShouldEqual, webrtc.SDPTypeAnswer)
			So(answer.SDP, ShouldNotBeBlank)
		})

		Convey("A candidate request is accepted", func() {
			candidate := webrtc.ICECandidateInit{
				Candidate: "candidate:1 1 UDP 2130706431 127.0.0.1 50000 typ host",
			}
			candBytes, err := json.Marshal(candidate)
			So(err, ShouldBeNil)

			req := httptest.NewRequest("POST", "/candidate", bytes.NewReader(candBytes))
			req.Header.Set("Content-Type", "application/json")
			res, err := hub.app.Test(req)
			So(err, ShouldBeNil)
			So(res.StatusCode, ShouldEqual, 200)
		})

		Convey("Run drains tee without error", func() {
			tee := NewWebRTCTee(t.Context(), "test-webrtc-tee", 16)
			tee.Transition(runtime.READY)
			defer func() { So(tee.Close(), ShouldBeNil) }()

			done := make(chan error, 1)
			runCtx, runCancel := context.WithCancel(ctx)
			go func() {
				// Use runCtx to shut down Run
				hubWithCancel := *hub
				hubWithCancel.System = runtime.NewSystem(runCtx, "hub-test")
				wrtcWithCancel := &WebRTC{hub: &hubWithCancel}
				done <- wrtcWithCancel.Run(tee)
			}()

			state := &types.ManifoldState{
				Version:     1,
				At:          time.Now(),
				GridX:       2,
				GridY:       2,
				GridZ:       2,
				GridSpacing: 1.0,
			}
			tee.Push(&data.Measurement[float64]{Source: "manifold", Result: state})

			time.Sleep(50 * time.Millisecond)
			runCancel()

			So(<-done, ShouldBeNil)
		})
	})
}
