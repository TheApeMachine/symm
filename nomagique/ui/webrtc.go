package ui

import (
	"context"
	"fmt"

	"github.com/gofiber/fiber/v3"
	"github.com/pion/webrtc/v4"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/runtime"
)

/*
WebRTC follows the simple data channel example from pion.
*/
type WebRTC struct {
	*runtime.System
	hub    *Hub
	api    *webrtc.API
	config webrtc.Configuration
}

/*
NewWebRTC configures the WebRTC transport.
If api is nil, a default pion WebRTC API is used.
If config is nil, default ICE servers and mux policy are used.
*/
func NewWebRTC(
	ctx context.Context,
	hub *Hub,
	api *webrtc.API,
	config *webrtc.Configuration,
) *WebRTC {
	if api == nil {
		api = webrtc.NewAPI()
	}

	if config == nil {
		config = &webrtc.Configuration{
			ICEServers: []webrtc.ICEServer{
				{URLs: []string{"stun:stun.l.google.com:19302"}},
			},
			BundlePolicy:  webrtc.BundlePolicyBalanced,
			RTCPMuxPolicy: webrtc.RTCPMuxPolicyRequire,
		}
	}

	rtc := &WebRTC{
		hub:    hub,
		api:    api,
		config: *config,
	}

	hub.app.Post("/webrtc/manifold", func(fiberCtx fiber.Ctx) (err error) {
		var offer webrtc.SessionDescription

		if err = fiberCtx.Bind().Body(&offer); err != nil {
			rtc.Error(errnie.Err(
				errnie.BadRequest,
				"[webrtc] failed to bind offer",
				err,
			))

			return fiber.ErrBadRequest
		}

		peerConn, err := rtc.api.NewPeerConnection(rtc.config)

		if err != nil {
			rtc.Error(errnie.Err(
				errnie.BadRequest,
				"[webrtc] failed to create peer connection",
				err,
			))

			return fiber.ErrBadRequest
		}

		setupICECandidateHandler(peerConn)
		setupDataChannelHandler(rtc, peerConn)

		if err := processOffer(rtc, peerConn, offer, fiberCtx); err != nil {
			return err
		}

		return nil
	})

	rtc.System = runtime.NewSystem(ctx, "webrtc", rtc)
	rtc.Transition(runtime.READY)

	return rtc
}

func setupICECandidateHandler(peerConn *webrtc.PeerConnection) {
	peerConn.OnICECandidate(func(candidate *webrtc.ICECandidate) {
		if candidate != nil {
			errnie.Info(fmt.Sprintf("[webrtc] new ICE candidate: %s", candidate.Address))
		}
	})
}

func setupDataChannelHandler(rtc *WebRTC, peerConn *webrtc.PeerConnection) {
	peerConn.OnDataChannel(func(dataChannel *webrtc.DataChannel) {
		dataChannel.OnOpen(func() {
			errnie.Info("[webrtc] data channel opened")
			rtc.hub.RegisterDataChannel(dataChannel)

			if sendErr := dataChannel.SendText("Hello from Go server 👋"); sendErr != nil {
				rtc.Error(errnie.Err(
					errnie.IO,
					"[webrtc] failed to send greeting text",
					sendErr,
				))
			}
		})

		dataChannel.OnMessage(func(message webrtc.DataChannelMessage) {
			errnie.Info(fmt.Sprintf("[webrtc] received: %s", string(message.Data)))
		})
	})
}

func processOffer(
	rtc *WebRTC,
	peerConn *webrtc.PeerConnection,
	offer webrtc.SessionDescription,
	fiberCtx fiber.Ctx,
) error {
	if err := peerConn.SetRemoteDescription(offer); err != nil {
		rtc.Error(errnie.Err(
			errnie.BadRequest,
			"[webrtc] failed to set remote description",
			err,
		))

		return fiber.ErrBadRequest
	}

	answer, err := peerConn.CreateAnswer(nil)

	if err != nil {
		rtc.Error(errnie.Err(
			errnie.Internal,
			"[webrtc] failed to create answer",
			err,
		))

		return fiber.ErrInternalServerError
	}

	gatherComplete := webrtc.GatheringCompletePromise(peerConn)

	if err := peerConn.SetLocalDescription(answer); err != nil {
		rtc.Error(errnie.Err(
			errnie.Internal,
			"[webrtc] failed to set local description",
			err,
		))

		return fiber.ErrInternalServerError
	}

	<-gatherComplete

	finalAnswer := peerConn.LocalDescription()

	if finalAnswer == nil {
		rtc.Error(errnie.Err(
			errnie.Internal,
			"[webrtc] local description is nil after ICE gathering",
			nil,
		))

		return fiber.ErrInternalServerError
	}

	return fiberCtx.JSON(*finalAnswer)
}
