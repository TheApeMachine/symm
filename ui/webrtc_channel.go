package ui

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"sync"

	"github.com/pion/sctp"
	"github.com/pion/webrtc/v4"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/types"
)

/*
fluidPeer owns the ordered data channels for one browser connection.
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

func (peer *fluidPeer) ready() bool {
	peer.mutex.RLock()
	defer peer.mutex.RUnlock()

	return peer.channels[types.ManifoldChannel] != nil
}

func (peer *fluidPeer) has(label string) bool {
	peer.mutex.RLock()
	defer peer.mutex.RUnlock()

	return peer.channels[label] != nil
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
	dataChannel   *webrtc.DataChannel
	drained       chan struct{}
	bufferedLimit uint64
	fail          func(error)
	startOnce     sync.Once

	// latest holds the freshest unsent record; latestMu guards it and
	// latestReady wakes the sender. frameID names the next logical frame.
	latestMu    sync.Mutex
	latest      []byte
	latestReady chan struct{}
	frameID     uint32
}

func newFluidChannel(
	ctx context.Context,
	dataChannel *webrtc.DataChannel,
	bufferedLimit uint64,
	fail func(error),
) *fluidChannel {
	ctx, cancel := context.WithCancel(ctx)
	channel := &fluidChannel{
		ctx: ctx, cancel: cancel, dataChannel: dataChannel,
		drained: make(chan struct{}, 1),
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

	select {
	case channel.latestReady <- struct{}{}:
	default:
	}
}

func (channel *fluidChannel) run() {
	for {
		select {
		case <-channel.ctx.Done():
			return
		case <-channel.latestReady:
			payload := channel.takeLatest()

			if payload == nil {
				continue
			}

			if err := channel.send(payload); err != nil {
				channel.failSend(err)
				return
			}
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
	if channel.ctx.Err() != nil ||
		(channel.dataChannel != nil &&
			channel.dataChannel.ReadyState() != webrtc.DataChannelStateOpen) {
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

	for offset, index := 0, uint32(0); offset < len(payload); offset += fluidSegmentSize {
		end := min(offset+fluidSegmentSize, len(payload))

		if err := channel.sendChunk(frameID, index, chunkCount, payload[offset:end]); err != nil {
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
) error {
	return channel.sendSegment(encodeFluidChunk(frameID, chunkIndex, chunkCount, payload))
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

func (channel *fluidChannel) sendSegment(segment []byte) error {
	for channel.dataChannel.BufferedAmount()+uint64(len(segment)) > channel.bufferedLimit {
		select {
		case <-channel.ctx.Done():
			return channel.ctx.Err()
		case <-channel.drained:
		}
	}

	return channel.dataChannel.Send(segment)
}

func (channel *fluidChannel) close() {
	channel.cancel()
	if channel.dataChannel != nil {
		_ = channel.dataChannel.Close()
	}
}
