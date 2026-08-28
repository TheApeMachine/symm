package ui

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/pion/webrtc/v4"
	"github.com/spf13/viper"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/logic/manifold"
	"github.com/theapemachine/symm/nomagique/runtime"
	"github.com/theapemachine/symm/telemetry"
	wire "github.com/theapemachine/symm/telemetry/generated/telemetry"
	"github.com/theapemachine/symm/types"
)

const (
	fluidRecordHeaderSize = 8
	// RFC 8831 recommends messages no larger than 16 KiB when SCTP message
	// interleaving is unavailable. Records are segmented and reassembled.
	fluidSegmentSize = 16 * 1024
)

var fluidRecordMagic = [4]byte{'S', 'F', 'D', '1'}

/*
FluidRTC owns the lossless, ordered WebRTC manifold publication plane.
*/
type FluidRTC struct {
	ctx           context.Context
	cancel        context.CancelFunc
	errMutex      sync.RWMutex
	err           error
	peersMutex    sync.RWMutex
	peers         map[*webrtc.PeerConnection]*fluidPeer
	bus           *runtime.Workspace
	consumerID    string
	queueLimit    int
	bufferedLimit uint64
	sequence      uint64
	ObserveModule func(string, time.Duration)
}

/*
NewFluidRTC configures the manifold transport without starting its Run loop.
*/
func NewFluidRTC(
	ctx context.Context,
	bus *runtime.Workspace,
	consumerID string,
) *FluidRTC {
	ctx, cancel := context.WithCancel(ctx)
	viper.SetDefault("ui.webrtc.client_queue_frames", 4)
	viper.SetDefault("ui.webrtc.buffered_segments", 64)
	queueLimit := viper.GetInt("ui.webrtc.client_queue_frames")
	bufferedSegments := viper.GetUint64("ui.webrtc.buffered_segments")
	fluidTransport := &FluidRTC{
		ctx:           ctx,
		cancel:        cancel,
		peers:         make(map[*webrtc.PeerConnection]*fluidPeer),
		bus:           bus,
		consumerID:    consumerID,
		queueLimit:    queueLimit,
		bufferedLimit: bufferedSegments * fluidSegmentSize,
	}

	if queueLimit < 1 || bufferedSegments < 1 {
		fluidTransport.err = fmt.Errorf(
			"webrtc: client_queue_frames and buffered_segments must be positive",
		)
	}

	if bus != nil {
		// *manifold.State publishes a full grid-field snapshot on every Hawkes-
		// triggered Step (several times a second); DeliveryLatestByKey holds
		// only the most recent value in a fixed cell instead of the default
		// 64K-slot ring, so an unread backlog never pins hours of past
		// snapshots in memory — only ever the current state matters here.
		runtime.RegisterSinkClass(
			bus,
			runtime.ServiceAnalytics, runtime.DeliveryLatestByKey, nil,
			func(state *manifold.State) {
				if state == nil {
					return
				}

				if !fluidTransport.HasChannel(types.ManifoldChannel) {
					return
				}

				_ = fluidTransport.publish(types.ManifoldChannel, fluidTransport.encodeManifold(state))
			},
		)

		runtime.RegisterSinkClass(
			bus,
			runtime.ServiceAnalytics, runtime.DeliveryLatestByKey, nil,
			func(payload []byte) {
				if len(payload) == 0 {
					return
				}

				if !fluidTransport.HasChannel(types.DiagnosticsChannel) {
					return
				}

				_ = fluidTransport.publish(types.DiagnosticsChannel, payload)
			},
		)
	}

	return fluidTransport
}

/*
encodeManifold builds the wire ManifoldFrame from the *manifold.State Step
returned, field for field. This is the transport boundary: the only place
sensorium.State and Reading are turned into bytes.
*/
func (fluidTransport *FluidRTC) encodeManifold(state *manifold.State) []byte {
	sequence := atomic.AddUint64(&fluidTransport.sequence, 1)

	modes := make([]*wire.WaveModeT, len(state.Modes))

	for index, mode := range state.Modes {
		modes[index] = &wire.WaveModeT{
			Omega:     mode.Omega,
			Real:      mode.Real,
			Imaginary: mode.Imag,
			Linewidth: mode.Linewidth,
		}
	}

	return telemetry.Encode(&wire.FrameT{
		Type: wire.FrameManifoldFrame,
		Value: &wire.ManifoldFrameT{
			Sequence:   sequence,
			N:          int64(state.N),
			Bytes:      state.Bytes,
			Seqs:       state.Seqs,
			TokenIds:   state.TokenIDs,
			ContentIds: state.ContentIDs,
			Phase:      state.Phase,
			Omega:      state.Omega,
			Energy:     state.Energy,
			Mass:       state.Mass,
			Heat:       state.Heat,
			Amp:        state.Amp,
			Pos:        state.Pos,
			Vel:        state.Vel,
			Clamped:    state.Clamped,
			Dark:       state.Dark,
			Reading: &wire.ManifoldReadingT{
				Divergence:       state.Reading.Divergence,
				GuidanceSpeed:    state.Reading.GuidanceSpeed,
				CoherenceMag2:    state.Reading.CoherenceMag2,
				PressureGradNorm: state.Reading.PressureGradNorm,
				ViscosityProxy:   state.Reading.ViscosityProxy,
				KuramotoR:        state.Reading.KuramotoR,
			},
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
		},
	})
}

func (fluidTransport *FluidRTC) Name() string { return "fluid-webrtc" }

func (fluidTransport *FluidRTC) Error() error {
	fluidTransport.errMutex.RLock()
	defer fluidTransport.errMutex.RUnlock()

	return fluidTransport.err
}

func (fluidTransport *FluidRTC) Active() bool {
	return fluidTransport.bus != nil
}

/*
Run drains direct manifold publications until shutdown or the first transport
failure. A live viewer never loses a queued state silently: when its bounded
queue fills, publication applies backpressure until the reliable channel drains.
*/
func (fluidTransport *FluidRTC) Run() error {
	if err := fluidTransport.Error(); err != nil {
		return err
	}

	<-fluidTransport.ctx.Done()

	return fluidTransport.Error()
}

/*
HasChannel reports whether any connected viewer owns the frame's named data
channel. Diagnostics and manifold viewers use separate peer connections.
*/
func (fluidTransport *FluidRTC) HasChannel(channel string) bool {
	fluidTransport.peersMutex.RLock()
	defer fluidTransport.peersMutex.RUnlock()

	for _, peer := range fluidTransport.peers {
		if peer.has(channel) {
			return true
		}
	}

	return false
}

/*
HasPeers reports whether a peer has registered every required domain channel.
*/
func (fluidTransport *FluidRTC) HasPeers() bool {
	fluidTransport.peersMutex.RLock()
	defer fluidTransport.peersMutex.RUnlock()

	for _, peer := range fluidTransport.peers {
		if peer.ready() {
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
		fluidTransport.queueLimit,
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

func (fluidTransport *FluidRTC) publish(channel string, payload []byte) error {
	started := time.Now()
	defer func() {
		if fluidTransport.ObserveModule != nil {
			fluidTransport.ObserveModule("webrtc-hub", time.Since(started))
		}
	}()

	fluidTransport.peersMutex.RLock()
	peers := make([]*fluidPeer, 0, len(fluidTransport.peers))
	for _, peer := range fluidTransport.peers {
		peers = append(peers, peer)
	}
	fluidTransport.peersMutex.RUnlock()

	for _, peer := range peers {
		if !peer.has(channel) {
			continue
		}

		_ = peer.enqueue(channel, payload)
	}

	return nil
}

func fluidError(message string, err error) error {
	return errnie.Err(errnie.IO, "webrtc: "+message, err)
}