package ui

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/gofiber/fiber/v3"
	flatbuffers "github.com/google/flatbuffers/go"
	"github.com/pion/webrtc/v4"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/runtime"
	"github.com/theapemachine/symm/telemetry/generated/telemetry"
	"github.com/theapemachine/symm/types"
)

type WebRTC struct {
	*runtime.System
	hub      *Hub
	pc       *webrtc.PeerConnection
	channels sync.Map
	tee      runtime.Tee
}

func NewWebRTC(ctx context.Context, hub *Hub, tee runtime.Tee) *WebRTC {
	wrtc := &WebRTC{
		hub: hub,
		tee: tee,
	}

	wrtc.Register()
	wrtc.System = runtime.NewSystem(ctx, "webrtc", wrtc)

	return wrtc
}

func (wrtc *WebRTC) Register() {
	wrtc.setupOfferHandler()
	wrtc.setupCandidateHandler()
	wrtc.setupStaticHandler()
}

func (wrtc *WebRTC) setupOfferHandler() {
	handler := func(fiberCtx fiber.Ctx) error {
		var offer webrtc.SessionDescription

		if err := fiberCtx.Bind().Body(&offer); err != nil {
			return fiber.ErrBadRequest
		}

		settingEngine := webrtc.SettingEngine{}
		settingEngine.SetIncludeLoopbackCandidate(true)

		api := webrtc.NewAPI(webrtc.WithSettingEngine(settingEngine))
		pc, err := api.NewPeerConnection(webrtc.Configuration{
			ICEServers: []webrtc.ICEServer{
				{URLs: []string{"stun:stun.l.google.com:19302"}},
			},
			BundlePolicy:  webrtc.BundlePolicyBalanced,
			RTCPMuxPolicy: webrtc.RTCPMuxPolicyRequire,
		})

		if err != nil {
			errnie.Error(errnie.Err(errnie.Internal, "webrtc: failed to create peer connection", err))
			return fiber.ErrInternalServerError
		}

		wrtc.pc = pc
		wrtc.setupICECandidateHandler(pc)
		wrtc.setupDataChannelHandler(pc)

		pc.OnConnectionStateChange(func(state webrtc.PeerConnectionState) {
			errnie.Info(fmt.Sprintf("webrtc: connection state changed to: %s", state.String()))
		})

		answer, err := wrtc.processOffer(pc, offer)

		if err != nil {
			errnie.Error(errnie.Err(errnie.Internal, "webrtc: failed to process offer", err))
			return fiber.ErrInternalServerError
		}

		return fiberCtx.JSON(answer)
	}

	wrtc.hub.app.Post("/offer", handler)
	wrtc.hub.app.Post("/webrtc/manifold", handler)
}

func (wrtc *WebRTC) setupICECandidateHandler(pc *webrtc.PeerConnection) {
	pc.OnICECandidate(func(candidate *webrtc.ICECandidate) {
		if candidate != nil {
			errnie.Info(fmt.Sprintf("webrtc: new ICE candidate: %s", candidate.Address))
		}
	})
}

func (wrtc *WebRTC) setupDataChannelHandler(pc *webrtc.PeerConnection) {
	pc.OnDataChannel(func(channel *webrtc.DataChannel) {
		label := channel.Label()

		channel.OnOpen(func() {
			errnie.Info(fmt.Sprintf("webrtc: data channel opened: %s", label))
			wrtc.channels.Store(channel, label)
		})

		channel.OnClose(func() {
			errnie.Info(fmt.Sprintf("webrtc: data channel closed: %s", label))
			wrtc.channels.Delete(channel)
		})

		if channel.ReadyState() == webrtc.DataChannelStateOpen {
			wrtc.channels.Store(channel, label)
		}
	})
}

func (wrtc *WebRTC) processOffer(
	pc *webrtc.PeerConnection,
	offer webrtc.SessionDescription,
) (*webrtc.SessionDescription, error) {
	if err := pc.SetRemoteDescription(offer); err != nil {
		return nil, err
	}

	answer, err := pc.CreateAnswer(nil)

	if err != nil {
		return nil, err
	}

	if err := pc.SetLocalDescription(answer); err != nil {
		return nil, err
	}

	gatherComplete := webrtc.GatheringCompletePromise(pc)
	select {
	case <-gatherComplete:
	case <-time.After(3 * time.Second):
		errnie.Warn("webrtc: ICE gathering timed out, returning gathered candidates")
	}

	finalAnswer := pc.LocalDescription()

	if finalAnswer == nil {
		return nil, fmt.Errorf("local description is nil after ICE gathering")
	}

	return finalAnswer, nil
}

func (wrtc *WebRTC) setupCandidateHandler() {
	wrtc.hub.app.Post("/candidate", func(fiberCtx fiber.Ctx) error {
		var candidate webrtc.ICECandidateInit

		if err := fiberCtx.Bind().Body(&candidate); err != nil {
			return fiber.ErrBadRequest
		}

		if wrtc.pc != nil {
			if err := wrtc.pc.AddICECandidate(candidate); err != nil {
				errnie.Error(err)
			}
		}

		return nil
	})
}

func (wrtc *WebRTC) setupStaticHandler() {
	wrtc.hub.app.Get("/", func(fiberCtx fiber.Ctx) error {
		return fiberCtx.SendFile("./demo.html")
	})
}

func (wrtc *WebRTC) Send(label string, payload []byte) {
	wrtc.channels.Range(func(key, value any) bool {
		if value.(string) != label {
			return true
		}

		dc := key.(*webrtc.DataChannel)

		if dc.ReadyState() == webrtc.DataChannelStateOpen {
			if err := dc.Send(payload); err != nil {
				errnie.Error(err)
			}
		}

		return true
	})
}

func (wrtc *WebRTC) Run() error {
	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-wrtc.hub.Context().Done():
			return nil
		case <-ticker.C:
			for pointer := wrtc.tee.Next(); pointer != nil; pointer = wrtc.tee.Next() {
				// UITee yields unsafe.Pointer to []byte containing MeasurementsFrameT flatbuffer.
				payload := *(*[]byte)(pointer)

				// Send the MeasurementsFrameT payload to the "telemetry" channel on WebRTC.
				wrtc.Send(types.ChannelTelemetry, payload)
			}
		}
	}
}

var webrtcBuilders = sync.Pool{
	New: func() any { return flatbuffers.NewBuilder(262144) },
}

func wrapMessage(msg *telemetry.MessageT) []byte {
	builder := webrtcBuilders.Get().(*flatbuffers.Builder)

	defer func() {
		builder.Reset()
		webrtcBuilders.Put(builder)
	}()

	offset := msg.Pack(builder)
	telemetry.FinishMessageBuffer(builder, offset)

	encoded := builder.FinishedBytes()
	frameBytes := make([]byte, len(encoded))
	copy(frameBytes, encoded)

	return frameBytes
}
