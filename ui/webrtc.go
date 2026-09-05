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
		fluidTransport.err = fmt.Errorf(
			"webrtc: buffered_segments must be positive",
		)
	}

	return fluidTransport
}

func (fluidTransport *FluidRTC) Name() string { return "fluid-webrtc" }

func (fluidTransport *FluidRTC) Error() error {
	fluidTransport.errMutex.RLock()
	defer fluidTransport.errMutex.RUnlock()

	return fluidTransport.err
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

	return fluidTransport.Error()
}

/*
Publish encodes one manifold advance into a ManifoldFrame and fans it to every
connected viewer that owns the manifold channel. A viewer without that channel
is skipped; a fully-booked channel returns an error so the caller can observe
backpressure rather than silently dropping a state.
*/
func (fluidTransport *FluidRTC) Publish(state *types.ManifoldState) error {
	if state == nil {
		return nil
	}

	sequence := fluidTransport.sequence.Add(1)
	payload := encodeManifold(state, sequence)

	return fluidTransport.publishBytes(types.ManifoldChannel, payload)
}

/*
PublishResonance fans one envelope's resonance artifact to every viewer owning
the resonance channel, wrapped in a lean EnvelopeState the frontend decodes
with the same EnvelopeState accessor it already uses for the websocket.
*/
func (fluidTransport *FluidRTC) PublishResonance(envelope *types.Envelope) error {
	if envelope == nil || envelope.Resonance == nil {
		return nil
	}

	state := &telemetry.EnvelopeStateT{
		Resonance: envelope.EncodeResonanceArtifactWire(),
	}
	payload := wrapStateFrame(state)

	return fluidTransport.publishBytes(types.ResonanceChannel, payload)
}

/*
PublishDiagnostics fans one envelope's ordered boundary trace to every viewer
owning the diagnostics channel, wrapped in a lean EnvelopeState carrying only
the boundaries the topology page ingests.
*/
func (fluidTransport *FluidRTC) PublishDiagnostics(envelope *types.Envelope) error {
	if envelope == nil {
		return nil
	}

	boundaries := envelope.EncodeBoundariesWire()

	if len(boundaries) == 0 {
		return nil
	}

	state := &telemetry.EnvelopeStateT{
		Boundaries: boundaries,
	}
	payload := wrapStateFrame(state)

	return fluidTransport.publishBytes(types.DiagnosticsChannel, payload)
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

		if channel == nil {
			continue
		}

		channel.enqueue(payload)
	}

	return nil
}

var webrtcBuilders = sync.Pool{
	New: func() any { return flatbuffers.NewBuilder(16384) },
}

/*
wrapStateFrame wraps a lean EnvelopeState mirror in the SYMM-identified Envelope
envelope the browser uses for every WebRTC channel, so resonance and diagnostics
share the manifold transport's framing and identifier.
*/
func wrapStateFrame(state *telemetry.EnvelopeStateT) []byte {
	builder := webrtcBuilders.Get().(*flatbuffers.Builder)

	defer func() {
		builder.Reset()
		webrtcBuilders.Put(builder)
	}()

	frame := &telemetry.EnvelopeStateFrameT{State: state}
	envelope := &telemetry.EnvelopeT{
		Frame: &telemetry.FrameT{
			Type:  telemetry.FrameEnvelopeStateFrame,
			Value: frame,
		},
	}

	offset := envelope.Pack(builder)
	telemetry.FinishEnvelopeBuffer(builder, offset)

	encoded := builder.FinishedBytes()
	frameBytes := make([]byte, len(encoded))
	copy(frameBytes, encoded)

	return frameBytes
}

/*
encodeManifold mirrors one *types.ManifoldState into the ManifoldFrame the
browser decodes, wrapped in the SYMM-identified Envelope the frontend's
decodeManifold expects: the resident sensorium State and Reading, the packed
Eulerian grid fields, and the spectral mode lattice, field for field.
*/
func encodeManifold(state *types.ManifoldState, sequence uint64) []byte {
	builder := webrtcBuilders.Get().(*flatbuffers.Builder)

	defer func() {
		builder.Reset()
		webrtcBuilders.Put(builder)
	}()

	reading := &telemetry.ManifoldReadingT{
		Divergence:       state.Reading.Divergence,
		GuidanceSpeed:    state.Reading.GuidanceSpeed,
		CoherenceMag2:    state.Reading.CoherenceMag2,
		PressureGradNorm: state.Reading.PressureGradNorm,
		ViscosityProxy:   state.Reading.ViscosityProxy,
		KuramotoR:        state.Reading.KuramotoR,
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

	frame := &telemetry.ManifoldFrameT{
		Sequence:      sequence,
		N:             int64(state.State.N),
		Bytes:         state.State.Bytes,
		Seqs:          state.State.Seqs,
		TokenIds:      state.State.TokenIDs,
		ContentIds:    state.State.ContentIDs,
		Phase:         state.State.Phase,
		Omega:         state.State.Omega,
		Energy:        state.State.Energy,
		Mass:          state.State.Mass,
		Heat:          state.State.Heat,
		Amp:           state.State.Amp,
		Pos:           state.State.Pos,
		Vel:           state.State.Vel,
		Clamped:       state.State.Clamped,
		Dark:          state.State.Dark,
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

	envelope := &telemetry.EnvelopeT{
		Sequence: sequence,
		Frame: &telemetry.FrameT{
			Type:  telemetry.FrameManifoldFrame,
			Value: frame,
		},
	}

	offset := envelope.Pack(builder)
	telemetry.FinishEnvelopeBuffer(builder, offset)

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
	peerConnection, err := webrtc.NewPeerConnection(webrtc.Configuration{})

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
		return webrtc.SessionDescription{}, fluidTransport.ctx.Err()
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
	return errnie.Err(errnie.IO, "webrtc: "+message, err)
}
