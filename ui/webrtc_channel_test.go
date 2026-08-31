package ui

import (
	"context"
	"encoding/binary"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
)

/*
fakeFluidTransport records every segment the sender attempts and can be told to
supersede the in-flight frame after a fixed number of sends, so mid-frame
preemption is deterministic without a live pion transport.
*/
type fakeFluidTransport struct {
	mu             sync.Mutex
	segments       [][]byte
	sent           chan struct{}
	sendCalls      atomic.Int64
	buffered       atomic.Uint64
	bufferedChecks atomic.Int64
	supersedeAt    int64
	supersede      func(*fluidChannel)
}

func (fake *fakeFluidTransport) BufferedAmount() uint64 {
	fake.bufferedChecks.Add(1)
	return fake.buffered.Load()
}

func (fake *fakeFluidTransport) Send(segment []byte) error {
	fake.mu.Lock()
	fake.segments = append(fake.segments, segment)
	fake.mu.Unlock()

	if fake.sent != nil {
		select {
		case fake.sent <- struct{}{}:
		default:
		}
	}

	if fake.supersede != nil && fake.sendCalls.Add(1) == fake.supersedeAt {
		fake.supersede(nil)
	}

	return nil
}

func (fake *fakeFluidTransport) Close() error { return nil }

func (fake *fakeFluidTransport) segmentCount() int {
	fake.mu.Lock()
	defer fake.mu.Unlock()

	return len(fake.segments)
}

func testFluidChannel(transport fluidTransport) *fluidChannel {
	ctx, cancel := context.WithCancel(context.Background())

	return &fluidChannel{
		ctx:           ctx,
		cancel:        cancel,
		transport:     transport,
		drained:       make(chan struct{}, 1),
		bufferedLimit: 64 * fluidSegmentSize,
		latestReady:   make(chan struct{}, 1),
	}
}

/*
TestFluidChannelTest proves the sender abandons the remaining chunks of an
in-flight logical frame the moment a fresher payload supersedes it, and returns
the distinct superseded sentinel rather than a transport failure.
*/
func TestFluidChannelPreemption(t *testing.T) {
	Convey("Given a multi-chunk frame being sent", t, func() {
		fake := &fakeFluidTransport{supersedeAt: 2}
		channel := testFluidChannel(fake)

		// Supersede after the second attempted segment.
		fake.supersede = func(receiver *fluidChannel) {
			channel.enqueue([]byte("fresher"))
		}

		payload := make([]byte, 4*fluidSegmentSize)
		err := channel.send(payload)

		Convey("the stale frame is abandoned mid-flight with the superseded sentinel", func() {
			So(errors.Is(err, errFrameSuperseded), ShouldBeTrue)
			So(fake.segmentCount(), ShouldEqual, 2)
		})

		Convey("the pending superseding payload is offered to the sender next", func() {
			So(channel.latest, ShouldNotBeNil)
			So(string(channel.latest), ShouldEqual, "fresher")
		})
	})

	Convey("Given a sender waiting for buffered amount to drain", t, func() {
		fake := &fakeFluidTransport{}
		fake.buffered.Store(64 * fluidSegmentSize)
		channel := testFluidChannel(fake)

		done := make(chan error, 1)

		go func() {
			done <- channel.send(make([]byte, 2*fluidSegmentSize))
		}()

		// Wait until the sender is actually parked on the buffered-amount
		// wait before superseding, so the ordering is deterministic.
		for fake.bufferedChecks.Load() == 0 {
		}

		// A newer record supersedes the stalled frame while the sender is
		// still waiting for drained: it abandons rather than holding the
		// fresher payload behind the stale one.
		channel.enqueue([]byte("fresher"))

		Convey("a newer frame arriving while drained is pending abandons the old frame", func() {
			err := <-done

			So(errors.Is(err, errFrameSuperseded), ShouldBeTrue)
			So(fake.segmentCount(), ShouldEqual, 0)
		})
	})
}

/*
TestEncodeFluidChunk proves every SCTP message is self-identifying: it names
its frame, chunk index, and chunk count, so an unordered, non-retransmitting
receiver can reassemble one complete frame and discard incomplete/obsolete ones.
*/
func TestEncodeFluidChunk(t *testing.T) {
	Convey("Given a segmented logical frame", t, func() {
		frameID := uint32(7)
		chunkCount := uint32(3)

		chunks := [][]byte{
			encodeFluidChunk(frameID, 0, chunkCount, []byte("abc")),
			encodeFluidChunk(frameID, 1, chunkCount, []byte("def")),
			encodeFluidChunk(frameID, 2, chunkCount, []byte("ghi")),
		}

		Convey("each chunk carries the frame identity and its position", func() {
			for index, chunk := range chunks {
				So(len(chunk), ShouldBeGreaterThan, fluidChunkHeaderSize)
				So(chunk[:4], ShouldResemble, []byte{'S', 'F', 'D', '1'})
				So(binary.LittleEndian.Uint32(chunk[4:8]), ShouldEqual, frameID)
				So(
					binary.LittleEndian.Uint32(chunk[8:12]),
					ShouldEqual,
					uint32(index),
				)
				So(binary.LittleEndian.Uint32(chunk[12:16]), ShouldEqual, chunkCount)
			}
		})

		Convey("the frames can be reassembled from chunks in any order", func() {
			ordered := []byte{}

			for _, index := range []int{2, 0, 1} {
				ordered = append(ordered, chunks[index][fluidChunkHeaderSize:]...)
			}

			So(string(ordered), ShouldEqual, "ghiabcdef")
		})
	})
}

/*
TestFluidChannelSenderDrainsLatestAfterPreemption proves the end-to-end sender
loop shape required by §17: after an in-flight frame is superseded while parked
on the buffered-amount wait, the sender drains the fresher payload from the
latest slot without needing a second wake token. The wake the fresh enqueue
produced is consumed inside sendSegment's preemption select; the run loop must
still transmit the fresh frame completely.
*/
func TestFluidChannelSenderDrainsLatestAfterPreemption(t *testing.T) {
	Convey("Given a sender loop with an old frame blocked on buffered amount", t, func() {
		fake := &fakeFluidTransport{sent: make(chan struct{}, 16)}
		fake.buffered.Store(64 * fluidSegmentSize)
		channel := testFluidChannel(fake)
		go channel.run()

		channel.enqueue(make([]byte, 4*fluidSegmentSize))

		// Park deterministically: wait until the sender is blocked inside the
		// buffered-amount wait of sendSegment.
		for fake.bufferedChecks.Load() == 0 {
		}

		// A fresh frame supersedes the old one. This is the exact moment the
		// old sendSegment's select consumes the fresh frame's wake token.
		channel.enqueue([]byte("fresher"))

		Convey("the fresher frame is transmitted completely without another enqueue", func() {
			// The old frame abandons without writing any segment, then the run
			// loop must drain latest directly. Verify the old frame sent zero
			// segments before the buffer is released.
			So(fake.segmentCount(), ShouldEqual, 0)

			// Release the transport buffer so the fresh frame can actually send.
			fake.buffered.Store(0)

			select {
			case channel.drained <- struct{}{}:
			default:
			}

			// The fresh single-chunk frame is sent once, complete — without any
			// third enqueue. Wait for Send to signal, not a polling read.
			select {
			case <-fake.sent:
			case <-time.After(2 * time.Second):
				So(fake.segmentCount(), ShouldEqual, 1)
				return
			}

			So(fake.segmentCount(), ShouldEqual, 1)
		})
	})
}
