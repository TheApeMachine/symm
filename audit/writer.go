package audit

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/bytedance/sonic"
	"github.com/theapemachine/datura/structure"
	"github.com/theapemachine/errnie"
)

/*
recorderCapacity is the lock-free ring depth. Power of two per MPMCRing's
contract. A full ring drops the event rather than stalling the producer, so
diagnostic recording can never apply backpressure to the hot path.
*/
const recorderCapacity = 1 << 16

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
	ring     *structure.MPMCRing[[]byte]
	done     chan struct{}
}

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

	ctx, cancel := context.WithCancel(context.Background())

	ring, err := structure.NewMPMCRing[[]byte](ctx, recorderCapacity)

	if err != nil {
		cancel()
		errnie.Error(fh.Close())

		return nil, err
	}

	recorder := &Recorder{
		ctx:      ctx,
		cancel:   cancel,
		filename: filename,
		fh:       fh,
		ring:     ring,
		done:     make(chan struct{}),
	}

	go recorder.drain()

	return recorder, nil
}

/*
Write marshals event and Push'es the encoded row onto the lock-free ring. It
never blocks: a full ring drops the row and reports it so the caller knows the
audit stream is saturated, but the hot path is never stalled.
*/
func (recorder *Recorder) Write(event any) error {
	if recorder == nil || recorder.ring == nil || recorder.ctx.Err() != nil {
		return errnie.Error(errnie.Err(
			errnie.IO,
			"audit: recorder is closed",
			nil,
		))
	}

	payload, err := sonic.Marshal(event)

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.IO,
			"audit: failed to marshal event",
			err,
		))
	}

	payload = append(payload, '\n')

	if !recorder.ring.Push(payload) {
		return errnie.Error(errnie.Err(
			errnie.IO,
			"audit: recorder ring is full, event dropped",
			nil,
		))
	}

	return nil
}

/*
drain is the single consumer. It owns the file handle, Pop's encoded rows off
the ring, and buffers writes to disk. It exits after the context is cancelled
and the ring is empty, flushing whatever remains.
*/
func (recorder *Recorder) drain() {
	defer close(recorder.done)

	writer := bufio.NewWriter(recorder.fh)

	flush := func() {
		if err := writer.Flush(); err != nil {
			errnie.Error(err)
		}
	}

	for {
		payload := recorder.ring.Pop()

		if payload == nil {
			select {
			case <-recorder.ctx.Done():
				// Drain any final rows the producers pushed before Close.
				for {
					remaining := recorder.ring.Pop()

					if remaining == nil {
						flush()
						return
					}

					if _, err := writer.Write(remaining); err != nil {
						errnie.Error(err)
					}
				}
			default:
				flush()
				time.Sleep(10 * time.Millisecond)
				continue
			}
		}

		if _, err := writer.Write(payload); err != nil {
			errnie.Error(err)
		}
	}
}

func (recorder *Recorder) Close() error {
	if recorder == nil {
		return errnie.Error(errnie.Err(
			errnie.IO,
			"audit: recorder is closed",
			nil,
		))
	}

	recorder.cancel()
	<-recorder.done

	fh := recorder.fh
	recorder.fh = nil

	if fh == nil {
		return nil
	}

	return fh.Close()
}
