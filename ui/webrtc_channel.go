package ui

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"sync"
	"sync/atomic"

	"github.com/pion/sctp"
	"github.com/pion/webrtc/v4"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/types"
)

/*
errFrameSuperseded is the sender's internal result for a logical frame that was
abandoned midway because a fresher payload arrived. It is not a transport
failure: the sender loop immediately picks up the newest pending record.
*/
var errFrameSuperseded = errors.New("fluid frame superseded")

/*
fluidPeer owns the per-viewer data channels for one browser connection. These
channels are unordered and non-retransmitting (see attach), despite the wording
below; the transport sets that policy at channel creation.
*/
type fluidPeer struct {
	ctx           context.Context
	fail          func(error)
	bufferedLimit uint64
	mutex         sync.RWMutex
	channels      map[string]*fluidChannel
}

func newFluidPeer(
	ctx context.Context,
	fail func(error),
	bufferedLimit uint64,
) *fluidPeer {
	return &fluidPeer{
		ctx: ctx, fail: fail, bufferedLimit: bufferedLimit,
		channels: make(map[string]*fluidChannel, 4),
	}
}

func (peer *fluidPeer) idle(label string) bool {
	peer.mutex.RLock()
	channel := peer.channels[label]
	peer.mutex.RUnlock()

	return channel != nil && channel.idle()
}

func (peer *fluidPeer) attach(dataChannel *webrtc.DataChannel) {
	label := dataChannel.Label()

	if label != types.ManifoldChannel &&
		label != types.ResonanceChannel &&
		label != types.DiagnosticsChannel {
		errnie.Error(fluidError("unsupported data channel "+label, nil))
		_ = dataChannel.Close()

		return
	}

	// These high-volume channels are replaceable live observations, not
	// durable event streams: unordered delivery with no retransmission lets
	// newer snapshots preempt stalled ones instead of head-of-line blocking a
	// peer behind one giant stale record.
	if dataChannel.Ordered() ||
		dataChannel.MaxRetransmits() == nil ||
		*dataChannel.MaxRetransmits() != 0 {
		errnie.Error(fluidError("data channels must be unordered with MaxRetransmits=0", nil))
		_ = dataChannel.Close()

		return
	}

	channel := newFluidChannel(
		peer.ctx,
		dataChannel,
		peer.bufferedLimit,
		peer.fail,
	)
	peer.mutex.Lock()
	previous := peer.channels[label]
	peer.channels[label] = channel
	peer.mutex.Unlock()

	if previous != nil {
		previous.close()
	}

	dataChannel.OnOpen(channel.start)
}

func (peer *fluidPeer) close() {
	peer.mutex.Lock()
	channels := peer.channels
	peer.channels = make(map[string]*fluidChannel, 4)
	peer.mutex.Unlock()

	for _, channel := range channels {
		channel.close()
	}
}

/*
fluidChannel serializes complete records through one unordered, non-
retransmitting SCTP channel.

Every channel is latest-wins: the manifold field advance, the resonance
artifact, and the diagnostics trace are all continuously-refreshed snapshots,
not durable event streams a viewer must replay in full. The publisher drops the
previous pending record into this single slot, the sender always drains the
freshest one, and neither can ever block the market pipeline or fill a queue
whose stale frames would only be shipped late. Each outgoing chunk is self-
identifying (frame ID, chunk index, chunk count), so the browser can reassemble
one complete frame, discard incomplete frames, and discard obsolete frames once
a newer frame is available.
*/
type fluidChannel struct {
	ctx           context.Context
	cancel        context.CancelFunc
	transport     fluidTransport
	drained       chan struct{}
	bufferedLimit uint64
	fail          func(error)
	startOnce     sync.Once

	// latest holds the freshest unsent record; latestMu guards it and
	// latestReady wakes the sender. frameID names the next logical frame and
	// sendGen increments whenever a newer record supersedes the one currently
	// being transmitted, so a stale in-flight frame is abandoned mid-chunk.
	latestMu    sync.Mutex
	latest      []byte
	latestReady chan struct{}
	frameID     uint32
	sendGen     atomic.Uint64

	// sending is true while the sender is transmitting a frame's chunks. It is
	// what idle() reports on, so a publisher can decline to encode a record
	// this channel could only supersede mid-flight.
	sending atomic.Bool
}

/*
idle reports that the channel has finished its last frame and holds nothing
pending, so a fresh record encoded now will be transmitted whole.

A large record is many SCTP chunks; the publisher that hands one over faster
than the channel can drain it does not merely waste the encode, it supersedes
every frame mid-flight and the viewer never reassembles a complete one. Asking
first is what keeps latest-wins from becoming never-wins.
*/
func (channel *fluidChannel) idle() bool {
	if channel.ctx.Err() != nil {
		return false
	}

	if channel.sending.Load() {
		return false
	}

	channel.latestMu.Lock()
	defer channel.latestMu.Unlock()

	return channel.latest == nil
}

/*
fluidTransport is the narrow SCTP-boundary slice the sender touches: buffered
bytes, send, and close. The live implementation is *webrtc.DataChannel; tests
inject a fake to drive deterministic mid-frame preemption.
*/
type fluidTransport interface {
	BufferedAmount() uint64
	Send([]byte) error
	Close() error
}

type dataChannelTransport struct {
	channel *webrtc.DataChannel
}

func (transport dataChannelTransport) BufferedAmount() uint64 {
	return transport.channel.BufferedAmount()
}

func (transport dataChannelTransport) Send(segment []byte) error {
	return transport.channel.Send(segment)
}

func (transport dataChannelTransport) Close() error {
	return transport.channel.Close()
}

func newFluidChannel(
	ctx context.Context,
	dataChannel *webrtc.DataChannel,
	bufferedLimit uint64,
	fail func(error),
) *fluidChannel {
	ctx, cancel := context.WithCancel(ctx)
	channel := &fluidChannel{
		ctx: ctx, cancel: cancel, transport: dataChannelTransport{channel: dataChannel},
		drained:       make(chan struct{}, 1),
		bufferedLimit: bufferedLimit, fail: fail,
		latestReady: make(chan struct{}, 1),
	}
	dataChannel.SetBufferedAmountLowThreshold(bufferedLimit - fluidSegmentSize)
	dataChannel.OnBufferedAmountLow(func() {
		select {
		case channel.drained <- struct{}{}:
		default:
		}
	})
	dataChannel.OnClose(cancel)
	dataChannel.OnError(func(err error) {
		if channel.ctx.Err() != nil ||
			dataChannel.ReadyState() != webrtc.DataChannelStateOpen {
			return
		}

		errnie.Error(fluidError("viewer data channel failed", err))
		cancel()
	})

	return channel
}

func (channel *fluidChannel) start() {
	channel.startOnce.Do(func() { go channel.run() })
}

/*
enqueue stores payload as the channel's latest-wins record and wakes the sender.
It never blocks and never errors: a fresher record simply replaces the pending
one, which is the only behaviour a live replaceable snapshot can want.
*/
func (channel *fluidChannel) enqueue(payload []byte) {
	if channel.ctx.Err() != nil {
		return
	}

	channel.latestMu.Lock()
	channel.latest = payload
	channel.latestMu.Unlock()

	// A fresher record supersedes any logical frame currently mid-flight: the
	// sender abandons the old frame's remaining chunks and starts the newest
	// one immediately.
	channel.sendGen.Add(1)

	select {
	case channel.latestReady <- struct{}{}:
	default:
	}
}

func (channel *fluidChannel) run() {
	for {
		// Wake semantics: a token means "there may be work", never "exactly
		// one frame". The sender drains the latest slot on every iteration
		// and only blocks when it is nil, so a superseded frame (whose wake
		// token was consumed inside sendSegment) can never strand the fresher
		// payload waiting for a second wake that will not arrive.
		payload := channel.takeLatest()

		if payload == nil {
			select {
			case <-channel.ctx.Done():
				return
			case <-channel.latestReady:
			}

			continue
		}

		channel.sending.Store(true)
		err := channel.send(payload)
		channel.sending.Store(false)

		if err != nil {
			if errors.Is(err, errFrameSuperseded) {
				// A fresher record is already resident in latest: loop back
				// and take it immediately, without waiting for another token.
				continue
			}

			channel.failSend(err)
			return
		}
	}
}

func (channel *fluidChannel) takeLatest() []byte {
	channel.latestMu.Lock()
	payload := channel.latest
	channel.latest = nil
	channel.latestMu.Unlock()

	return payload
}

func (channel *fluidChannel) failSend(err error) {
	if channel.ctx.Err() != nil || channel.transport == nil {
		return
	}

	if errors.Is(err, sctp.ErrPayloadDataStateNotExist) {
		channel.close()
		return
	}

	if channel.fail != nil {
		channel.fail(fluidError("unable to send record", err))
	}

	channel.close()
}

func (channel *fluidChannel) send(payload []byte) error {
	if len(payload) > math.MaxUint32 {
		return fmt.Errorf("fluid publication exceeds uint32 transport record")
	}

	channel.frameID++

	frameID := channel.frameID
	chunkCount := uint32((len(payload) + fluidSegmentSize - 1) / fluidSegmentSize)
	generation := channel.sendGen.Load()

	for offset, index := 0, uint32(0); offset < len(payload); offset += fluidSegmentSize {
		if channel.sendGen.Load() != generation {
			return errFrameSuperseded
		}

		end := min(offset+fluidSegmentSize, len(payload))

		if err := channel.sendChunk(frameID, index, chunkCount, payload[offset:end], generation); err != nil {
			return err
		}

		index++
	}

	return nil
}

/*
sendChunk writes one self-identifying chunk: magic + frameID + chunkIndex +
chunkCount followed by the chunk payload. Every chunk names its frame and its
position, so an unordered, non-retransmitting receiver can reassemble one
complete frame and discard any incomplete/obsolete frame the moment a newer
frame is observed.
*/
func (channel *fluidChannel) sendChunk(
	frameID uint32,
	chunkIndex uint32,
	chunkCount uint32,
	payload []byte,
	generation uint64,
) error {
	return channel.sendSegment(
		encodeFluidChunk(frameID, chunkIndex, chunkCount, payload),
		generation,
	)
}

/*
encodeFluidChunk builds one self-identifying chunk message without touching the
transport, so the framing is testable in isolation.
*/
func encodeFluidChunk(
	frameID uint32,
	chunkIndex uint32,
	chunkCount uint32,
	payload []byte,
) []byte {
	segment := make([]byte, fluidChunkHeaderSize+len(payload))
	copy(segment[:4], fluidRecordMagic[:])
	binary.LittleEndian.PutUint32(segment[4:8], frameID)
	binary.LittleEndian.PutUint32(segment[8:12], chunkIndex)
	binary.LittleEndian.PutUint32(segment[12:16], chunkCount)
	copy(segment[fluidChunkHeaderSize:], payload)

	return segment
}

func (channel *fluidChannel) sendSegment(segment []byte, generation uint64) error {
	for channel.transport.BufferedAmount()+uint64(len(segment)) > channel.bufferedLimit {
		// A newer record may arrive while the sender waits for the transport
		// to drain; abandon the stale frame rather than holding the fresher
		// one behind it.
		if channel.sendGen.Load() != generation {
			return errFrameSuperseded
		}

		select {
		case <-channel.ctx.Done():
			return channel.ctx.Err()
		case <-channel.drained:
		case <-channel.latestReady:
			if channel.sendGen.Load() != generation {
				return errFrameSuperseded
			}
		}
	}

	return channel.transport.Send(segment)
}

func (channel *fluidChannel) close() {
	channel.cancel()

	if channel.transport != nil {
		_ = channel.transport.Close()
	}
}
