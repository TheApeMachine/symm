package ui

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"sync"
	"time"

	"github.com/pion/webrtc/v4"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/types"
)

const (
	fluidRecordHeaderSize = 8
	// RFC 8831 recommends messages no larger than 16 KiB when SCTP message
	// interleaving is unavailable. Ordered records are reassembled unchanged.
	fluidSegmentSize = 16 * 1024
)

var fluidRecordMagic = [4]byte{'S', 'F', 'D', '1'}

/*
FluidRTC owns the WebRTC peers consuming direct Fields, Particle, and Phase
publications from the manifold solver.
*/
type FluidRTC struct {
	ctx    context.Context
	cancel context.CancelFunc
	mutex  sync.RWMutex
	peers  map[*webrtc.PeerConnection]*fluidPeer
}

/*
NewFluidRTC creates an empty WebRTC manifold transport.
*/
func NewFluidRTC(ctx context.Context) *FluidRTC {
	ctx, cancel := context.WithCancel(ctx)

	return &FluidRTC{
		ctx:    ctx,
		cancel: cancel,
		peers:  make(map[*webrtc.PeerConnection]*fluidPeer),
	}
}

/*
HasPeers checks whether any WebRTC peer connection is actively attached.
*/
func (transport *FluidRTC) HasPeers() bool {
	transport.mutex.RLock()
	defer transport.mutex.RUnlock()

	return len(transport.peers) > 0
}

/*
Run drains direct manifold publications and fans each one to the data channel
it names. A frame carries its own destination because the solver knows which
view it built the frame for; recovering that here by sniffing the payload's
leading bytes only re-derived it, and tied routing to the encoding.
*/
func (transport *FluidRTC) Run(publications <-chan types.FluidFrame) {
	for {
		select {
		case <-transport.ctx.Done():
			return
		case frame, open := <-publications:
			if !open {
				return
			}

			if !transport.HasPeers() {
				time.Sleep(50 * time.Millisecond)
				continue
			}

			transport.publish(frame.Channel, frame.Payload)
		}
	}
}

/*
Answer accepts one browser offer and returns a complete non-trickle answer.
*/
func (transport *FluidRTC) Answer(
	offer webrtc.SessionDescription,
) (webrtc.SessionDescription, error) {
	peerConnection, err := webrtc.NewPeerConnection(webrtc.Configuration{})

	if err != nil {
		return webrtc.SessionDescription{}, errnie.Error(errnie.Err(
			errnie.IO,
			"webrtc: unable create new peer connection - "+err.Error(),
			err,
		))
	}

	peer := newFluidPeer(transport.ctx)
	transport.add(peerConnection, peer)
	peerConnection.OnDataChannel(peer.attach)

	peerConnection.OnConnectionStateChange(func(state webrtc.PeerConnectionState) {
		if state == webrtc.PeerConnectionStateFailed || state == webrtc.PeerConnectionStateClosed {
			transport.remove(peerConnection)
		}
	})

	if err = peerConnection.SetRemoteDescription(offer); err != nil {
		transport.remove(peerConnection)

		return webrtc.SessionDescription{}, errnie.Error(errnie.Err(
			errnie.IO,
			"webrtc: unable to set remove description - "+err.Error(),
			err,
		))
	}

	answer, err := peerConnection.CreateAnswer(nil)

	if err != nil {
		transport.remove(peerConnection)

		return webrtc.SessionDescription{}, errnie.Error(errnie.Err(
			errnie.IO,
			"webrtc: unable to create answer - "+err.Error(),
			err,
		))
	}

	gathered := webrtc.GatheringCompletePromise(peerConnection)

	if err = peerConnection.SetLocalDescription(answer); err != nil {
		transport.remove(peerConnection)

		return webrtc.SessionDescription{}, errnie.Error(errnie.Err(
			errnie.IO,
			"webrtc: unable to set local description - "+err.Error(),
			err,
		))
	}

	select {
	case <-transport.ctx.Done():
		transport.remove(peerConnection)
		return webrtc.SessionDescription{}, transport.ctx.Err()
	case <-gathered:
	}

	local := peerConnection.LocalDescription()

	if local == nil {
		transport.remove(peerConnection)

		return webrtc.SessionDescription{}, errnie.Error(errnie.Err(
			errnie.IO,
			"webrtc: peer connection has no local description",
			err,
		))
	}

	return *local, nil
}

/*
Close terminates every fluid peer and stops publication fanout.
*/
func (transport *FluidRTC) Close() error {
	transport.cancel()
	transport.mutex.Lock()
	peers := transport.peers
	transport.peers = make(map[*webrtc.PeerConnection]*fluidPeer)
	transport.mutex.Unlock()
	var err error

	for peerConnection, peer := range peers {
		peer.close()
		err = errors.Join(err, peerConnection.Close())
	}

	return err
}

func (transport *FluidRTC) add(
	peerConnection *webrtc.PeerConnection,
	peer *fluidPeer,
) {
	transport.mutex.Lock()
	transport.peers[peerConnection] = peer
	transport.mutex.Unlock()
}

func (transport *FluidRTC) remove(peerConnection *webrtc.PeerConnection) {
	transport.mutex.Lock()
	peer := transport.peers[peerConnection]
	delete(transport.peers, peerConnection)
	transport.mutex.Unlock()

	if peer != nil {
		peer.close()
	}

	_ = peerConnection.Close()
}

func (transport *FluidRTC) publish(channel string, payload []byte) {
	transport.mutex.RLock()
	peers := make([]*fluidPeer, 0, len(transport.peers))

	for _, peer := range transport.peers {
		peers = append(peers, peer)
	}

	transport.mutex.RUnlock()

	for _, peer := range peers {
		peer.enqueue(channel, payload)
	}
}

type fluidPeer struct {
	ctx      context.Context
	mutex    sync.RWMutex
	channels map[string]*fluidChannel
}

func newFluidPeer(ctx context.Context) *fluidPeer {
	return &fluidPeer{
		ctx:      ctx,
		channels: make(map[string]*fluidChannel, 3),
	}
}

func (peer *fluidPeer) attach(dataChannel *webrtc.DataChannel) {
	label := dataChannel.Label()

	if label != types.FluidFieldsChannel &&
		label != types.FluidParticlesChannel &&
		label != types.FluidPhaseChannel {
		errnie.Error(errnie.Err(
			errnie.Validation,
			"webrtc: unsupported fluid data channel "+label,
			nil,
		))
		_ = dataChannel.Close()
		return
	}

	if !dataChannel.Ordered() || dataChannel.MaxPacketLifeTime() != nil ||
		dataChannel.MaxRetransmits() != nil {

		errnie.Error(errnie.Err(
			errnie.Validation,
			"webrtc: fluid data channels must be ordered and reliable",
			nil,
		))

		_ = dataChannel.Close()
		return
	}

	dataChannel.OnOpen(func() {
		channel := newFluidChannel(peer.ctx, dataChannel)
		peer.mutex.Lock()
		previous := peer.channels[label]
		peer.channels[label] = channel
		peer.mutex.Unlock()

		if previous != nil {
			previous.close()
		}

		go channel.run()
	})
}

func (peer *fluidPeer) enqueue(channel string, payload []byte) {
	peer.mutex.RLock()
	writer := peer.channels[channel]
	peer.mutex.RUnlock()

	if writer != nil {
		writer.enqueue(payload)
	}
}

func (peer *fluidPeer) close() {
	peer.mutex.Lock()
	channels := peer.channels
	peer.channels = make(map[string]*fluidChannel, 3)
	peer.mutex.Unlock()

	for _, channel := range channels {
		channel.close()
	}
}

type fluidChannel struct {
	ctx         context.Context
	cancel      context.CancelFunc
	dataChannel *webrtc.DataChannel
	pending     chan []byte
	drained     chan struct{}
}

func newFluidChannel(
	ctx context.Context,
	dataChannel *webrtc.DataChannel,
) *fluidChannel {
	ctx, cancel := context.WithCancel(ctx)
	channel := &fluidChannel{
		ctx:         ctx,
		cancel:      cancel,
		dataChannel: dataChannel,
		pending:     make(chan []byte, 1),
		drained:     make(chan struct{}, 1),
	}

	dataChannel.SetBufferedAmountLowThreshold(0)
	dataChannel.OnBufferedAmountLow(func() {
		select {
		case channel.drained <- struct{}{}:
		default:
		}
	})

	dataChannel.OnClose(cancel)
	dataChannel.OnError(func(err error) {
		errnie.Error(errnie.Err(errnie.IO, "fluid data channel failed: "+err.Error(), err))
		cancel()
	})

	return channel
}

func (channel *fluidChannel) enqueue(payload []byte) {
	select {
	case channel.pending <- payload:
		return
	default:
	}

	select {
	case <-channel.pending:
	default:
	}

	select {
	case channel.pending <- payload:
	case <-channel.ctx.Done():
	}
}

func (channel *fluidChannel) run() {
	for {
		select {
		case <-channel.ctx.Done():
			return
		case payload, open := <-channel.pending:
			if !open {
				return
			}

			if err := channel.send(payload); err != nil {
				errnie.Error(errnie.Err(errnie.IO, "send fluid record: "+err.Error(), err))
				channel.cancel()
				return
			}

			channel.waitUntilDrained()
		}
	}
}

func (channel *fluidChannel) send(payload []byte) error {
	if len(payload) > math.MaxUint32 {
		return fmt.Errorf("fluid publication exceeds uint32 transport record")
	}

	header := make([]byte, fluidRecordHeaderSize)
	copy(header[:4], fluidRecordMagic[:])
	binary.LittleEndian.PutUint32(header[4:8], uint32(len(payload)))

	if err := channel.dataChannel.Send(header); err != nil {
		return err
	}

	for offset := 0; offset < len(payload); offset += fluidSegmentSize {
		end := min(offset+fluidSegmentSize, len(payload))

		if err := channel.dataChannel.Send(payload[offset:end]); err != nil {
			return err
		}
	}

	return nil
}

func (channel *fluidChannel) waitUntilDrained() {
	for channel.dataChannel.BufferedAmount() > 0 {
		select {
		case <-channel.ctx.Done():
			return
		case <-channel.drained:
		}
	}
}

func (channel *fluidChannel) close() {
	channel.cancel()
	_ = channel.dataChannel.Close()
}
