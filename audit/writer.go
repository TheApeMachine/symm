package audit

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/bytedance/sonic"
	"github.com/klauspost/compress/zstd"
	"github.com/theapemachine/datura/structure"
	"github.com/theapemachine/symm/types"
)

/*
recorderCapacity is the lock-free ring depth. Power of two per MPMCRing's
contract. A full ring drops the event rather than stalling the producer, so
audit recording can never apply backpressure to the hot path.
*/
const recorderCapacity = 1 << 16

/*
overflowInterval is how often the consumer emits one coalesced overflow row
while drops accumulate, independent of whether the ring is empty.
*/
const overflowInterval = 100 * time.Millisecond

/*
Recorder is a generic jsonl data recorder. Producers Push encoded events onto a
lock-free MPMC ring from any goroutine; a single background goroutine owns the
file handle and drains the ring, so no producer ever blocks on file I/O or a
mutex. This mirrors the rest of the system's lock-free single-consumer wiring.
*/
type Recorder struct {
	ctx      context.Context
	cancel   context.CancelFunc
	filename string
	fh       *os.File
	encoder  *zstd.Encoder
	ring     *structure.MPMCRing[[]byte]
	done     chan struct{}
	dropped  atomic.Uint64
	inFlight atomic.Int64
	closing  atomic.Bool
	closed   atomic.Bool
	asyncMu  sync.Mutex
	asyncErr error
	closeMu  sync.Mutex
}

/*
NewRecorder opens the jsonl file and starts the single drain consumer.
*/
func NewRecorder(filename string) (*Recorder, error) {
	if filename == "" {
		return nil, fmt.Errorf("audit: filename is required")
	}

	if err := os.MkdirAll(filepath.Dir(filename), 0o755); err != nil {
		return nil, err
	}

	fh, err := os.OpenFile(
		filename,
		os.O_CREATE|os.O_WRONLY|os.O_APPEND,
		0o644,
	)

	if err != nil {
		return nil, err
	}

	var encoder *zstd.Encoder

	if strings.HasSuffix(filename, ".zst") {
		encoder, err = zstd.NewWriter(
			fh,
			zstd.WithEncoderLevel(zstd.SpeedFastest),
			zstd.WithEncoderConcurrency(1),
		)

		if err != nil {
			_ = fh.Close()
			return nil, fmt.Errorf("audit: create zstd writer: %w", err)
		}
	}

	ctx, cancel := context.WithCancel(context.Background())

	ring, err := structure.NewMPMCRing[[]byte](ctx, recorderCapacity)

	if err != nil {
		cancel()

		if encoder != nil {
			_ = encoder.Close()
		}

		_ = fh.Close()

		return nil, err
	}

	recorder := &Recorder{
		ctx:      ctx,
		cancel:   cancel,
		filename: filename,
		fh:       fh,
		encoder:  encoder,
		ring:     ring,
		done:     make(chan struct{}),
	}

	go recorder.drain()

	return recorder, nil
}

/*
Write marshals event and Push'es the encoded row onto the lock-free ring. It
never blocks: a full ring drops the row and returns SaturatedError so callers
can count loss without hot-path logging. A closing gate rejects new writes once
Close begins so accepted producers finish before drain quiesces.
*/
func (recorder *Recorder) Write(event any) error {
	if recorder == nil || recorder.ring == nil || recorder.closing.Load() {
		return types.ClosedError{Component: "audit"}
	}

	recorder.inFlight.Add(1)
	defer recorder.inFlight.Add(-1)

	if recorder.closing.Load() {
		return types.ClosedError{Component: "audit"}
	}

	payload, err := sonic.Marshal(event)

	if err != nil {
		return fmt.Errorf("audit: marshal: %w", err)
	}

	payload = append(payload, '\n')

	if !recorder.ring.Push(payload) {
		recorder.dropped.Add(1)

		return types.SaturatedError{Component: "audit"}
	}

	return nil
}

/*
drain is the single consumer. It owns the file handle, Pop's encoded rows off
the ring, and buffers writes to disk. Overflow is emitted on a bounded timer so
sustained saturation still records loss. It exits after cancel and quiescence.
*/
func (recorder *Recorder) drain() {
	defer close(recorder.done)

	var destination io.Writer = recorder.fh

	if recorder.encoder != nil {
		destination = recorder.encoder
	}

	writer := bufio.NewWriter(destination)
	pendingDropped := uint64(0)
	ticker := time.NewTicker(overflowInterval)
	defer ticker.Stop()

	recordOverflow := func() {
		pendingDropped += recorder.dropped.Swap(0)

		if pendingDropped == 0 {
			return
		}

		payload, err := sonic.Marshal(map[string]any{
			"channel": "operational",
			"type":    "audit_overflow",
			"value": map[string]any{
				"dropped": pendingDropped,
			},
		})

		if err != nil {
			recorder.retainErr(err)
			return
		}

		payload = append(payload, '\n')

		if _, err = writer.Write(payload); err != nil {
			recorder.retainErr(err)
			return
		}

		pendingDropped = 0
	}

	flush := func() {
		if err := writer.Flush(); err != nil {
			recorder.retainErr(err)
		}

		if recorder.encoder != nil {
			if err := recorder.encoder.Flush(); err != nil {
				recorder.retainErr(err)
			}
		}
	}
	closeEncoder := func() {
		if recorder.encoder == nil {
			return
		}

		if err := recorder.encoder.Close(); err != nil {
			recorder.retainErr(err)
		}
	}

	for {
		payload, ok := recorder.ring.Pop()

		if !ok || payload == nil {
			select {
			case <-recorder.ctx.Done():
				for {
					remaining, drained := recorder.ring.Pop()

					if !drained || remaining == nil {
						recordOverflow()
						flush()
						closeEncoder()
						return
					}

					if _, err := writer.Write(remaining); err != nil {
						recorder.retainErr(err)
					}
				}
			case <-ticker.C:
				recordOverflow()
				flush()
			default:
				time.Sleep(10 * time.Millisecond)
			}

			continue
		}

		if _, err := writer.Write(payload); err != nil {
			recorder.retainErr(err)
		}
	}
}

/*
retainErr keeps the first asynchronous I/O failure for Close to return.
A dedicated mutex preserves first-error-wins across mixed concrete error types
where atomic.Value CompareAndSwap cannot compare interface equality reliably.
*/
func (recorder *Recorder) retainErr(err error) {
	if err == nil {
		return
	}

	recorder.asyncMu.Lock()
	defer recorder.asyncMu.Unlock()

	if recorder.asyncErr == nil {
		recorder.asyncErr = err
	}
}

/*
Close stops new writers, waits for in-flight producers, drains to quiescence,
fsyncs, and returns the first retained asynchronous error joined with close.
Idempotent and safe under concurrent Close.
*/
func (recorder *Recorder) Close() error {
	if recorder == nil {
		return types.ClosedError{Component: "audit"}
	}

	recorder.closeMu.Lock()

	if recorder.closed.Load() {
		err := recorder.combinedErr(nil)
		recorder.closeMu.Unlock()

		return err
	}

	recorder.closing.Store(true)

	for recorder.inFlight.Load() > 0 {
		time.Sleep(time.Millisecond)
	}

	recorder.cancel()
	recorder.closeMu.Unlock()

	<-recorder.done

	recorder.closeMu.Lock()
	defer recorder.closeMu.Unlock()

	if recorder.closed.Load() {
		return recorder.combinedErr(nil)
	}

	var syncErr error
	fh := recorder.fh
	recorder.fh = nil

	if fh != nil {
		syncErr = fh.Sync()
		closeErr := fh.Close()

		if syncErr == nil {
			syncErr = closeErr
		}
	}

	recorder.closed.Store(true)

	return recorder.combinedErr(syncErr)
}

/*
combinedErr joins the first async drain error with a close-path error.
*/
func (recorder *Recorder) combinedErr(closeErr error) error {
	recorder.asyncMu.Lock()
	async := recorder.asyncErr
	recorder.asyncMu.Unlock()

	switch {
	case async == nil:
		return closeErr
	case closeErr == nil:
		return async
	default:
		return fmt.Errorf("%w; close: %v", async, closeErr)
	}
}
