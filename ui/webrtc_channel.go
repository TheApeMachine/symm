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
	queueLimit    int
	bufferedLimit uint64
	mutex         sync.RWMutex
	channels      map[string]*fluidChannel
}

func newFluidPeer(
	ctx context.Context,
	fail func(error),
	queueLimit int,
	bufferedLimit uint64,
) *fluidPeer {
	return &fluidPeer{
		ctx: ctx, fail: fail, queueLimit: queueLimit, bufferedLimit: bufferedLimit,
		channels: make(map[string]*fluidChannel, 3),
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
		label != types.DiagnosticsChannel {
		errnie.Error(fluidError("unsupported data channel "+label, nil))
		_ = dataChannel.Close()
		return
	}

	if !dataChannel.Ordered() || dataChannel.MaxPacketLifeTime() != nil ||
		dataChannel.MaxRetransmits() != nil {
		errnie.Error(fluidError("data channels must be ordered and reliable", nil))
		_ = dataChannel.Close()
		return
	}

	channel := newFluidChannel(
		peer.ctx,
		dataChannel,
		peer.queueLimit,
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
	peer.channels = make(map[string]*fluidChannel, 3)
	peer.mutex.Unlock()

	for _, channel := range channels {
		channel.close()
	}
}

/*
fluidChannel serializes complete records through one reliable SCTP channel.
*/
type fluidChannel struct {
	ctx           context.Context
	cancel        context.CancelFunc
	dataChannel   *webrtc.DataChannel
	pending       chan []byte
	drained       chan struct{}
	bufferedLimit uint64
	fail          func(error)
	startOnce     sync.Once
}

func newFluidChannel(
	ctx context.Context,
	dataChannel *webrtc.DataChannel,
	queueLimit int,
	bufferedLimit uint64,
	fail func(error),
) *fluidChannel {
	ctx, cancel := context.WithCancel(ctx)
	channel := &fluidChannel{
		ctx: ctx, cancel: cancel, dataChannel: dataChannel,
		pending: make(chan []byte, queueLimit), drained: make(chan struct{}, 1),
		bufferedLimit: bufferedLimit, fail: fail,
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

func (channel *fluidChannel) run() {
	for {
		select {
		case <-channel.ctx.Done():
			return
		case payload := <-channel.pending:
			if err := channel.send(payload); err != nil {
				channel.failSend(err)
				return
			}
		}
	}
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

	var header [fluidRecordHeaderSize]byte
	copy(header[:4], fluidRecordMagic[:])
	binary.LittleEndian.PutUint32(header[4:], uint32(len(payload)))

	if err := channel.sendSegment(header[:]); err != nil {
		return err
	}

	for offset := 0; offset < len(payload); offset += fluidSegmentSize {
		end := min(offset+fluidSegmentSize, len(payload))

		if err := channel.sendSegment(payload[offset:end]); err != nil {
			return err
		}
	}

	return nil
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
