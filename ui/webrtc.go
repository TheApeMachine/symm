package ui

import (
	"context"
	"errors"
	"fmt"
	"github.com/theapemachine/symm/nomagique/runtime"
	"sync"

	"github.com/pion/webrtc/v4"
	"github.com/spf13/viper"
	"github.com/theapemachine/errnie"
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
	publications  *runtime.Channel[types.FluidFrame]
	consumerID    string
	queueLimit    int
	bufferedLimit uint64
}

/*
NewFluidRTC configures the manifold transport without starting its Run loop.
*/
func NewFluidRTC(
	ctx context.Context,
	publications *runtime.Channel[types.FluidFrame],
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
		publications:  publications,
		consumerID:    consumerID,
		queueLimit:    queueLimit,
		bufferedLimit: bufferedSegments * fluidSegmentSize,
	}

	if queueLimit < 1 || bufferedSegments < 1 {
		fluidTransport.err = fmt.Errorf(
			"webrtc: client_queue_frames and buffered_segments must be positive",
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

func (fluidTransport *FluidRTC) Active() bool {
	return fluidTransport.publications != nil
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

	if fluidTransport.publications == nil {
		return nil
	}

	fluidTransport.publications.Subscribe(
		fluidTransport.consumerID,
		func(frame types.FluidFrame) error {
			if !fluidTransport.HasChannel(frame.Channel) {
				return nil
			}

			if err := fluidTransport.publish(frame.Channel, frame.Payload); err != nil {
				fluidTransport.fail(err)

				return err
			}

			return nil
		},
	)

	<-fluidTransport.ctx.Done()

	return fluidTransport.Error()

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
		fluidTransport.fail,
		fluidTransport.queueLimit,
		fluidTransport.bufferedLimit,
	)
	fluidTransport.add(peerConnection, peer)
	peerConnection.OnDataChannel(peer.attach)
	peerConnection.OnConnectionStateChange(func(state webrtc.PeerConnectionState) {
		if state == webrtc.PeerConnectionStateFailed || state == webrtc.PeerConnectionStateClosed {
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

func (fluidTransport *FluidRTC) fail(err error) {
	if err == nil {
		return
	}

	fluidTransport.errMutex.Lock()

	if fluidTransport.err == nil {
		fluidTransport.err = fluidError("publication transport failed", err)
	}

	fluidTransport.errMutex.Unlock()
	fluidTransport.cancel()
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

	_ = peerConnection.Close()
}

func (fluidTransport *FluidRTC) publish(channel string, payload []byte) error {
	fluidTransport.peersMutex.RLock()
	defer fluidTransport.peersMutex.RUnlock()

	for _, peer := range fluidTransport.peers {
		if !peer.has(channel) {
			continue
		}

		if err := peer.enqueue(channel, payload); err != nil && !errors.Is(err, context.Canceled) {
			return err
		}
	}

	return nil
}

func fluidError(message string, err error) error {
	return errnie.Err(errnie.IO, "webrtc: "+message, err)
}
