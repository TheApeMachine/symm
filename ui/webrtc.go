package ui

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	flatbuffers "github.com/google/flatbuffers/go"
	"github.com/pion/webrtc/v4"
	"github.com/spf13/viper"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/telemetry/generated/telemetry"
	"github.com/theapemachine/symm/types"
)

const (
	// fluidChunkHeaderSize is the per-SCTP-message framing: magic(4) +
	// frameID(4) + chunkIndex(4) + chunkCount(4). Every chunk is self-
	// identifying, so unordered, non-retransmitting channels can reassemble
	// one complete frame and discard obsolete/incomplete ones.
	fluidChunkHeaderSize = 16
	// RFC 8831 recommends messages no larger than 16 KiB when SCTP message
	// interleaving is unavailable. Records are segmented and reassembled.
	fluidSegmentSize = 16 * 1024
)

var fluidRecordMagic = [4]byte{'S', 'F', 'D', '1'}

/*
FluidRTC owns the unordered, non-retransmitting WebRTC publication plane for
the manifold, resonance, and diagnostics channels. Every channel is
latest-wins, so the transport never queues a backlog of stale snapshots and
never blocks the market pipeline.
*/
type FluidRTC struct {
	ctx           context.Context
	cancel        context.CancelFunc
	errMutex      sync.RWMutex
	err           error
	peersMutex    sync.RWMutex
	peers         map[*webrtc.PeerConnection]*fluidPeer
	consumerID    string
	bufferedLimit uint64
	sequence      atomic.Uint64
	ObserveModule func(string, time.Duration)
}

/*
NewFluidRTC configures the manifold transport without starting its Run loop.
*/
func NewFluidRTC(
	ctx context.Context,
	consumerID string,
) *FluidRTC {
	ctx, cancel := context.WithCancel(ctx)
	viper.SetDefault("ui.webrtc.buffered_segments", 64)
	bufferedSegments := viper.GetUint64("ui.webrtc.buffered_segments")
	fluidTransport := &FluidRTC{
		ctx:           ctx,
		cancel:        cancel,
		peers:         make(map[*webrtc.PeerConnection]*fluidPeer),
		consumerID:    consumerID,
		bufferedLimit: bufferedSegments * fluidSegmentSize,
	}

	if bufferedSegments < 1 {
		fluidTransport.err = errnie.Error(fmt.Errorf(
			"webrtc: buffered_segments must be positive",
		))
	}

	return fluidTransport
}

func (fluidTransport *FluidRTC) Name() string { return "fluid-webrtc" }

func (fluidTransport *FluidRTC) Error() error {
	fluidTransport.errMutex.RLock()
	defer fluidTransport.errMutex.RUnlock()

	return errnie.Error(fluidTransport.err)
}

/*
Run drains direct manifold publications until shutdown or the first transport
failure. Observer snapshots are replaceable: a bounded latest-wins boundary
means a slow viewer receives a fresher replaceable state and, on a feed
failure, the transport fails explicitly rather than silently losing frames.
Durable historical truth lives in Hindsight/raw capture, never in this path.
*/
func (fluidTransport *FluidRTC) Run() error {
	if err := fluidTransport.Error(); err != nil {
		return err
	}

	<-fluidTransport.ctx.Done()

	return errnie.Error(fluidTransport.Error())
}

/*
Publish encodes one manifold advance into a ManifoldFrame and fans it to every
connected viewer that owns the manifold channel. A viewer without that channel
is skipped; a fully-booked channel returns an error so the caller can observe
backpressure rather than silently dropping a state.
*/
func (fluidTransport *FluidRTC) Publish(state *types.ManifoldState) error {
	if state == nil || !fluidTransport.Wants(types.ManifoldChannel) {
		return nil
	}

	sequence := fluidTransport.sequence.Add(1)
	payload := encodeManifold(state, sequence)

	return fluidTransport.publishBytes(types.ManifoldChannel, payload)
}

/*
WantsManifold reports whether any connected viewer owns the manifold channel
and is ready to receive another frame. It satisfies manifold.Viewer.
*/
func (fluidTransport *FluidRTC) WantsManifold() bool {
	return fluidTransport.Wants(types.ManifoldChannel)
}

/*
PublishManifold satisfies manifold.Viewer by serializing one manifold advance
into a ManifoldFrame flatbuffer and broadcasting it across the manifold WebRTC
data channel.
*/
func (fluidTransport *FluidRTC) PublishManifold(state *types.ManifoldState) {
	if err := fluidTransport.Publish(state); err != nil {
		errnie.Error(err)
	}
}

/*
PublishResonance fans one resonance artifact to every viewer owning
the resonance channel, wrapped in a canonical ResonanceFrame.
*/
func (fluidTransport *FluidRTC) PublishResonance(artifact *types.ResonanceArtifact) error {
	if artifact == nil || !fluidTransport.Wants(types.ResonanceChannel) {
		return nil
	}

	wireRow := artifact.EncodeWire()

	if wireRow == nil {
		return nil
	}

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

	return errnie.Error(fluidTransport.publishBytes(types.ResonanceChannel, payload))
}

/*
publishBytes fans one encoded record to every viewer that owns the named
channel. Every channel is latest-wins, so a busy viewer receives the freshest
record and the market pipeline is never blocked or error-flooded.
*/
func (fluidTransport *FluidRTC) publishBytes(channelName string, payload []byte) error {
	fluidTransport.peersMutex.RLock()
	defer fluidTransport.peersMutex.RUnlock()

	for _, peer := range fluidTransport.peers {
		peer.mutex.RLock()
		channel := peer.channels[channelName]
		peer.mutex.RUnlock()

		if channel == nil || !channel.idle() {
			continue
		}

		channel.enqueue(payload)
	}

	return nil
}

var webrtcBuilders = sync.Pool{
	New: func() any { return flatbuffers.NewBuilder(262144) },
}

/*
wrapMessage wraps a message in the SYMM-identified Message buffer the browser
uses for every WebRTC channel, so resonance and manifold share the transport's
framing and identifier.
*/
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

/*
encodeManifold mirrors one *types.ManifoldState into the ManifoldFrame the
browser decodes, wrapped in the SYMM-identified Message the frontend's
decodeManifold expects: the resident sensorium State and Reading, the packed
Eulerian grid fields, and the spectral mode lattice, field for field.
*/
func encodeManifold(state *types.ManifoldState, sequence uint64) []byte {
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

	var n int64
	var bytes, seqs, tokenIds, contentIds []int64
	var phase, omega, energy, mass, heat, amp, pos, vel []float32
	var clamped, dark []bool

	if state.State != nil {
		n = int64(state.State.N)
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
		Sequence:      sequence,
		At:            state.At.UnixNano(),
		Version:       state.Version,
		N:             n,
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
	}

	msg := &telemetry.MessageT{
		Sequence: sequence,
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

/*
Wants reports whether any connected viewer owns the named channel AND is ready
for another frame. Every publisher asks this before encoding: the encode is the
expensive half of a publication, and a record handed to a channel still
draining the previous one is both wasted work and — for a multi-chunk record —
a frame the viewer can never reassemble.
*/
func (fluidTransport *FluidRTC) Wants(channel string) bool {
	fluidTransport.peersMutex.RLock()
	defer fluidTransport.peersMutex.RUnlock()

	for _, peer := range fluidTransport.peers {
		if peer.idle(channel) {
			return true
		}
	}

	return false
}

/*
Answer accepts one browser offer and returns a complete non-trickle answer.
*/
func (fluidTransport *FluidRTC) Answer(
	offer webrtc.SessionDescription,
) (webrtc.SessionDescription, error) {
	settingEngine := webrtc.SettingEngine{}
	settingEngine.SetIncludeLoopbackCandidate(true)

	if err := settingEngine.SetAnsweringDTLSRole(webrtc.DTLSRoleServer); err != nil {
		return webrtc.SessionDescription{}, fluidError(
			"unable to set answering DTLS role",
			err,
		)
	}

	api := webrtc.NewAPI(webrtc.WithSettingEngine(settingEngine))
	peerConnection, err := api.NewPeerConnection(webrtc.Configuration{})

	if err != nil {
		return webrtc.SessionDescription{}, fluidError(
			"unable to create peer connection",
			err,
		)
	}

	peer := newFluidPeer(
		fluidTransport.ctx,
		func(err error) {
			fluidTransport.remove(peerConnection)
		},
		fluidTransport.bufferedLimit,
	)
	fluidTransport.add(peerConnection, peer)
	peerConnection.OnDataChannel(peer.attach)
	peerConnection.OnConnectionStateChange(func(state webrtc.PeerConnectionState) {
		if state == webrtc.PeerConnectionStateFailed || state == webrtc.PeerConnectionStateClosed || state == webrtc.PeerConnectionStateDisconnected {
			fluidTransport.remove(peerConnection)
		}
	})

	if err = peerConnection.SetRemoteDescription(offer); err != nil {
		fluidTransport.remove(peerConnection)
		return webrtc.SessionDescription{}, fluidError("unable to set remote description", err)
	}

	answer, err := peerConnection.CreateAnswer(nil)

	if err != nil {
		fluidTransport.remove(peerConnection)
		return webrtc.SessionDescription{}, fluidError("unable to create answer", err)
	}

	gathered := webrtc.GatheringCompletePromise(peerConnection)

	if err = peerConnection.SetLocalDescription(answer); err != nil {
		fluidTransport.remove(peerConnection)
		return webrtc.SessionDescription{}, fluidError("unable to set local description", err)
	}

	select {
	case <-fluidTransport.ctx.Done():
		fluidTransport.remove(peerConnection)
		return webrtc.SessionDescription{}, errnie.Error(fluidTransport.ctx.Err())
	case <-gathered:
	}

	local := peerConnection.LocalDescription()

	if local == nil {
		fluidTransport.remove(peerConnection)
		return webrtc.SessionDescription{}, fluidError("peer connection has no local description", nil)
	}

	return *local, nil
}

/*
Close terminates every peer and stops publication fanout.
*/
func (fluidTransport *FluidRTC) Close() error {
	fluidTransport.cancel()
	fluidTransport.peersMutex.Lock()
	peers := fluidTransport.peers
	fluidTransport.peers = make(map[*webrtc.PeerConnection]*fluidPeer)
	fluidTransport.peersMutex.Unlock()
	var err error

	for peerConnection, peer := range peers {
		peer.close()
		err = errors.Join(err, peerConnection.Close())
	}

	return err
}

func (fluidTransport *FluidRTC) add(
	peerConnection *webrtc.PeerConnection,
	peer *fluidPeer,
) {
	fluidTransport.peersMutex.Lock()
	fluidTransport.peers[peerConnection] = peer
	fluidTransport.peersMutex.Unlock()
}

func (fluidTransport *FluidRTC) remove(peerConnection *webrtc.PeerConnection) {
	fluidTransport.peersMutex.Lock()
	peer := fluidTransport.peers[peerConnection]
	delete(fluidTransport.peers, peerConnection)
	fluidTransport.peersMutex.Unlock()

	if peer != nil {
		peer.close()
	}

	if peerConnection != nil {
		_ = peerConnection.Close()
	}
}

func fluidError(message string, err error) error {
	return errnie.Error(errnie.Err(errnie.IO, "webrtc: "+message, err))
}
