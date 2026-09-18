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
	hub *Hub
}

/*
NewWebRTC configures the WebRTC transport.
*/
func NewWebRTC(
	ctx context.Context,
	hub *Hub,
) *WebRTC {
	rtc := &WebRTC{
		hub: hub,
	}

	var pc *webrtc.PeerConnection

	hub.app.Post("/webrtc/manifold", func(c fiber.Ctx) (err error) {
		var offer webrtc.SessionDescription

		if err = c.Bind().Body(&offer); err != nil {
			rtc.Error(errnie.Err(
				errnie.BadRequest,
				"[webrtc] failed to bind offer",
				err,
			))

			return fiber.ErrBadRequest
		}

		// PeerConnection with enhanced configuration for better browser compatibility
		pc, err = webrtc.NewPeerConnection(webrtc.Configuration{
			ICEServers: []webrtc.ICEServer{
				{URLs: []string{"stun:stun.l.google.com:19302"}},
			},
			BundlePolicy:  webrtc.BundlePolicyBalanced,
			RTCPMuxPolicy: webrtc.RTCPMuxPolicyRequire,
		})

		if err != nil {
			rtc.Error(errnie.Err(
				errnie.BadRequest,
				"[webrtc] failed to create peer connection",
				err,
			))

			return fiber.ErrBadRequest
		}

		setupICECandidateHandler(pc)
		setupDataChannelHandler(rtc, pc)

		if err := processOffer(rtc, pc, offer, c); err != nil {
			return err
		}

		return nil
	})

	setupCandidateHandler(rtc, &pc)

	rtc.System = runtime.NewSystem(ctx, "webrtc", rtc)
	rtc.Transition(runtime.READY)

	return rtc
}

func setupICECandidateHandler(pc *webrtc.PeerConnection) {
	pc.OnICECandidate(func(c *webrtc.ICECandidate) {
		if c != nil {
			errnie.Info(fmt.Sprintf("[webrtc] new ICE candidate: %s", c.Address))
		}
	})
}

func setupDataChannelHandler(rtc *WebRTC, pc *webrtc.PeerConnection) {
	pc.OnDataChannel(func(d *webrtc.DataChannel) {
		d.OnOpen(func() {
			errnie.Info("[webrtc] data channel opened")

			if sendErr := d.SendText("Hello from Go server 👋"); sendErr != nil {
				rtc.Error(errnie.Err(
					errnie.IO,
					"[webrtc] failed to send greeting text",
					sendErr,
				))
			}
		})

		d.OnMessage(func(msg webrtc.DataChannelMessage) {
			errnie.Info(fmt.Sprintf("[webrtc] received: %s", string(msg.Data)))
		})
	})
}

func processOffer(
	rtc *WebRTC,
	pc *webrtc.PeerConnection,
	offer webrtc.SessionDescription,
	c fiber.Ctx,
) error {
	if err := pc.SetRemoteDescription(offer); err != nil {
		rtc.Error(errnie.Err(
			errnie.BadRequest,
			"[webrtc] failed to set remote description",
			err,
		))

		return fiber.ErrBadRequest
	}

	answer, err := pc.CreateAnswer(nil)

	if err != nil {
		rtc.Error(errnie.Err(
			errnie.Internal,
			"[webrtc] failed to create answer",
			err,
		))

		return fiber.ErrInternalServerError
	}

	if err := pc.SetLocalDescription(answer); err != nil {
		rtc.Error(errnie.Err(
			errnie.Internal,
			"[webrtc] failed to set local description",
			err,
		))

		return fiber.ErrInternalServerError
	}

	gatherComplete := webrtc.GatheringCompletePromise(pc)
	<-gatherComplete

	finalAnswer := pc.LocalDescription()

	if finalAnswer == nil {
		rtc.Error(errnie.Err(
			errnie.Internal,
			"[webrtc] local description is nil after ICE gathering",
			nil,
		))

		return fiber.ErrInternalServerError
	}

	return c.JSON(*finalAnswer)
}

func setupCandidateHandler(rtc *WebRTC, pc **webrtc.PeerConnection) {
	rtc.hub.app.Post("/candidate", func(c fiber.Ctx) error {
		var candidate webrtc.ICECandidateInit

		if err := c.Bind().Body(&candidate); err != nil {
			rtc.Error(errnie.Err(
				errnie.BadRequest,
				"[webrtc] failed to bind ICE candidate",
				err,
			))

			return fiber.ErrBadRequest
		}

		if *pc != nil {
			if err := (*pc).AddICECandidate(candidate); err != nil {
				rtc.Error(errnie.Err(
					errnie.BadRequest,
					"[webrtc] failed to add ICE candidate",
					err,
				))

				return fiber.ErrBadRequest
			}
		}

		return nil
	})
}
