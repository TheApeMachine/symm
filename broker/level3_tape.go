package broker

import (
	"bytes"
	"fmt"
	"sync"
	"sync/atomic"

	"github.com/google/uuid"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/system"
)

/*
objectSink accepts whole objects for asynchronous upload.
*/
type objectSink interface {
	WriteAtPrefix(key string, value []byte) (int, error)
}

/*
Level3Tape batches verbatim venue Level3 JSON frames into NDJSON parts on S3.
*/
type Level3Tape struct {
	sink   objectSink
	runID  string
	epoch  uint64
	part   atomic.Uint64
	mu     sync.Mutex
	buf    bytes.Buffer
	lines  int
	closed atomic.Bool
}

/*
NewLevel3Tape constructs a tape that writes under market/level3/{run}/{epoch}/.
*/
func NewLevel3Tape(sink objectSink, runID string) *Level3Tape {
	if runID == "" {
		runID = uuid.NewString()
	}

	return &Level3Tape{
		sink:  sink,
		runID: runID,
	}
}

/*
Record appends one verbatim websocket text frame as an NDJSON line.
*/
func (tape *Level3Tape) Record(raw []byte) {
	if !system.Cfg.Market.Level3.Record || len(raw) == 0 || tape.closed.Load() {
		return
	}

	tape.mu.Lock()
	defer tape.mu.Unlock()

	if tape.closed.Load() {
		return
	}

	tape.buf.Write(raw)

	if raw[len(raw)-1] != '\n' {
		tape.buf.WriteByte('\n')
	}

	tape.lines++

	if tape.lines >= system.Cfg.Market.Level3.RecordFlushLines ||
		tape.buf.Len() >= system.Cfg.Market.Level3.RecordFlushBytes {
		tape.flushLocked()
	}
}

/*
Flush writes any buffered frames into the object sink.
*/
func (tape *Level3Tape) Flush() {
	tape.mu.Lock()
	defer tape.mu.Unlock()

	tape.flushLocked()
}

/*
Close flushes once and refuses further records.
*/
func (tape *Level3Tape) Close() {
	tape.mu.Lock()
	defer tape.mu.Unlock()

	tape.flushLocked()
	tape.closed.Store(true)
}

func (tape *Level3Tape) flushLocked() {
	if tape.buf.Len() == 0 {
		return
	}

	part := tape.part.Add(1) - 1
	key := fmt.Sprintf(
		"market/level3/%s/%d/%019d.ndjson",
		tape.runID, tape.epoch, part,
	)
	body := append([]byte(nil), tape.buf.Bytes()...)

	tape.buf.Reset()
	tape.lines = 0

	if _, err := tape.sink.WriteAtPrefix(key, body); err != nil {
		errnie.Error(errnie.Err(
			errnie.IO,
			"[level3.tape] upload "+key,
			err,
		))
	}
}
