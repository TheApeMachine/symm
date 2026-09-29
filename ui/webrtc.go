package ui

import (
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
	hub      *Hub
	pc       *webrtc.PeerConnection
	channels sync.Map
}

func NewWebRTC(hub *Hub) *WebRTC {
	wrtc := &WebRTC{
		hub: hub,
	}

	wrtc.Register()

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

func (wrtc *WebRTC) Run(tee *WebRTCTee) error {
	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-wrtc.hub.Context().Done():
			return nil
		case <-ticker.C:
			if tee.Status() != runtime.READY {
				continue
			}

			for pointer := tee.Next(); pointer != nil; pointer = tee.Next() {
				switch artifact := (*(*any)(pointer)).(type) {
				case *types.ManifoldState:
					payload := encodeManifold(artifact)
					wrtc.Send(types.ManifoldChannel, payload)
				case *types.ResonanceArtifact:
					wireRow := artifact.EncodeWire(types.Allows(artifact.Symbol))

					if wireRow != nil {
						frame := &telemetry.ResonanceFrameT{
							Rows: []*telemetry.ResonanceT{wireRow},
						}
						msg := &telemetry.MessageT{
							Frame: &telemetry.FrameT{
								Type:  telemetry.FrameResonanceFrame,
								Value: frame,
							},
						}
						payload := wrapMessage(msg)
						wrtc.Send(types.ResonanceChannel, payload)
					}
				}
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

func encodeManifold(state *types.ManifoldState) []byte {
	builder := webrtcBuilders.Get().(*flatbuffers.Builder)

	defer func() {
		builder.Reset()
		webrtcBuilders.Put(builder)
	}()

	health := &telemetry.PhysicsHealthT{
		Integrator: &telemetry.IntegratorHealthT{
			ContactDt:    state.Reading.Health.Integrator.ContactDT,
			RequestedDt:  state.Reading.Health.Integrator.RequestedDT,
			TargetDt:     state.Reading.Health.Integrator.TargetDT,
			AcceptedDt:   state.Reading.Health.Integrator.AcceptedDT,
			LastDt:       state.Reading.Health.Integrator.LastDT,
			MinDt:        state.Reading.Health.Integrator.MinDT,
			Time:         state.Reading.Health.Integrator.Time,
			HyperbolicDt: state.Reading.Health.Integrator.HyperbolicDT,
			ViscousDt:    state.Reading.Health.Integrator.ViscousDT,
			ThermalDt:    state.Reading.Health.Integrator.ThermalDT,
			ParticleDt:   state.Reading.Health.Integrator.ParticleDT,
			PhaseDt:      state.Reading.Health.Integrator.PhaseDT,
			CombinedDt:   state.Reading.Health.Integrator.CombinedDT,
			Substeps:     int32(state.Reading.Health.Integrator.Substeps),
			Rejections:   int32(state.Reading.Health.Integrator.Rejections),
		},
		Gas: &telemetry.GasHealthT{
			Mass:           state.Reading.Health.Gas.Mass,
			Internal:       state.Reading.Health.Gas.Internal,
			Kinetic:        state.Reading.Health.Gas.Kinetic,
			Total:          state.Reading.Health.Gas.Total,
			Momentum:       state.Reading.Health.Gas.Momentum[:],
			MinDensity:     state.Reading.Health.Gas.MinDensity,
			MinPressure:    state.Reading.Health.Gas.MinPressure,
			MinTemperature: state.Reading.Health.Gas.MinTemperature,
			MaxSpeed:       state.Reading.Health.Gas.MaxSpeed,
			MaxSound:       state.Reading.Health.Gas.MaxSound,
			MaxMach:        state.Reading.Health.Gas.MaxMach,
			VorticityRms:   state.Reading.Health.Gas.VorticityRMS,
			VorticityMax:   state.Reading.Health.Gas.VorticityMax,
			StrainRms:      state.Reading.Health.Gas.StrainRMS,
			StrainMax:      state.Reading.Health.Gas.StrainMax,
			ViscousPower:   state.Reading.Health.Gas.ViscousPower,
		},
		Wave: &telemetry.WaveHealthT{
			Norm:           state.Reading.Health.Wave.Norm,
			Kinetic:        state.Reading.Health.Wave.Kinetic,
			Potential:      state.Reading.Health.Wave.Potential,
			Nonlinear:      state.Reading.Health.Wave.Nonlinear,
			Chemical:       state.Reading.Health.Wave.Chemical,
			ProjectedNorm:  state.Reading.Health.Wave.ProjectedNorm,
			PhasePotential: state.Reading.Health.Wave.PhasePotential,
		},
		Pilot: &telemetry.PilotHealthT{
			DensityP01:          state.Reading.Health.Pilot.DensityP01,
			DensityP10:          state.Reading.Health.Pilot.DensityP10,
			DensityMedian:       state.Reading.Health.Pilot.DensityMedian,
			IntegrationErrorMax: state.Reading.Health.Pilot.IntegrationErrorMax,
			SpeedRms:            state.Reading.Health.Pilot.SpeedRMS,
			SpeedMax:            state.Reading.Health.Pilot.SpeedMax,
			DisplacementRms:     state.Reading.Health.Pilot.DisplacementRMS,
			DisplacementMax:     state.Reading.Health.Pilot.DisplacementMax,
			MinDensity:          state.Reading.Health.Pilot.MinDensity,
		},
		Sources: &telemetry.SourceLedgerT{
			GasEnergyResidual:        state.Reading.Health.Sources.GasEnergyResidual,
			ConservativeWaveError:    state.Reading.Health.Sources.ConservativeWaveError,
			PicDepositEnergyResidual: state.Reading.Health.Sources.PICDepositEnergyResidual,
			ParticleBalanceResidual:  state.Reading.Health.Sources.ParticleBalanceResidual,
			GravityBalanceResidual:   state.Reading.Health.Sources.GravityBalanceResidual,
		},
		ParticleThermal:       state.Reading.Health.ParticleThermal,
		ParticleOscillator:    state.Reading.Health.ParticleOscillator,
		ParticleKinetic:       state.Reading.Health.ParticleKinetic,
		ParticleMaterialTotal: state.Reading.Health.ParticleMaterialTotal,
	}

	reading := &telemetry.ManifoldReadingT{
		Divergence:       state.Reading.Divergence,
		GuidanceSpeed:    state.Reading.GuidanceSpeed,
		CoherenceMag2:    state.Reading.CoherenceMag2,
		PressureGradNorm: state.Reading.PressureGradNorm,
		ViscosityProxy:   state.Reading.ViscosityProxy,
		KuramotoR:        state.Reading.KuramotoR,
		KuramotoPsi:      state.Reading.KuramotoPsi,
		Health:           health,
	}

	modes := make([]*telemetry.WaveModeT, len(state.Modes))

	for index, mode := range state.Modes {
		modes[index] = &telemetry.WaveModeT{
			Omega:     mode.Omega,
			Real:      mode.Real,
			Imaginary: mode.Imag,
			Linewidth: mode.Linewidth,
		}
	}

	resultants := make([]*telemetry.PhaseResultantT, len(state.Resultants))

	for index, resultant := range state.Resultants {
		resultants[index] = &telemetry.PhaseResultantT{
			Side:           resultant.Side,
			Count:          int32(resultant.Count),
			TotalAmplitude: resultant.TotalAmplitude,
			Coherence:      resultant.Coherence,
			Phase:          resultant.Phase,
		}
	}

	var particleCount int64
	var bytes, seqs, tokenIds, contentIds []int64
	var phase, omega, energy, mass, heat, amp, pos, vel []float32
	var clamped, dark []bool

	if state.State != nil {
		particleCount = int64(state.State.N)
		bytes = state.State.Bytes
		seqs = state.State.Seqs
		tokenIds = state.State.TokenIDs
		contentIds = state.State.ContentIDs
		phase = state.State.Phase
		omega = state.State.Omega
		energy = state.State.Energy
		mass = state.State.Mass
		heat = state.State.Heat
		amp = state.State.Amp
		pos = state.State.Pos
		vel = state.State.Vel
		clamped = state.State.Clamped
		dark = state.State.Dark
	}

	frame := &telemetry.ManifoldFrameT{
		At:            state.At.UnixNano(),
		Version:       state.Version,
		N:             particleCount,
		Bytes:         bytes,
		Seqs:          seqs,
		TokenIds:      tokenIds,
		ContentIds:    contentIds,
		Phase:         phase,
		Omega:         omega,
		Energy:        energy,
		Mass:          mass,
		Heat:          heat,
		Amp:           amp,
		Pos:           pos,
		Vel:           vel,
		Clamped:       clamped,
		Dark:          dark,
		Reading:       reading,
		GridX:         int32(state.GridX),
		GridY:         int32(state.GridY),
		GridZ:         int32(state.GridZ),
		GridSpacing:   state.GridSpacing,
		MomRho:        state.MomRho,
		FieldEnergy:   state.FieldEnergy,
		WaveReal:      state.WaveReal,
		WaveImag:      state.WaveImag,
		DensityScale:  state.DensityScale,
		MomentumScale: state.MomentumScale,
		EnergyScale:   state.EnergyScale,
		WaveScale:     state.WaveScale,
		Modes:         modes,
		Resultants:    resultants,
	}

	msg := &telemetry.MessageT{
		Frame: &telemetry.FrameT{
			Type:  telemetry.FrameManifoldFrame,
			Value: frame,
		},
	}

	offset := msg.Pack(builder)
	telemetry.FinishMessageBuffer(builder, offset)

	encoded := builder.FinishedBytes()
	frameBytes := make([]byte, len(encoded))
	copy(frameBytes, encoded)

	return frameBytes
}
