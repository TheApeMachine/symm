package broker

import (
	"sync/atomic"
	"time"

	"github.com/theapemachine/symm/audit"
)

/*
DeadLetter records discarded or malformed desk events without blocking the tick loop.
*/
type DeadLetter struct {
	audit *audit.Writer
	drops atomic.Uint64
}

func NewDeadLetter(auditWriter *audit.Writer) *DeadLetter {
	return &DeadLetter{
		audit: auditWriter,
	}
}

func (deadLetter *DeadLetter) Drops() uint64 {
	if deadLetter == nil {
		return 0
	}

	return deadLetter.drops.Load()
}

func (deadLetter *DeadLetter) Record(kind string, reason string, detail map[string]any) {
	if deadLetter == nil {
		return
	}

	deadLetter.drops.Add(1)

	frame := audit.DeadLetterFrame{
		RecordedAt: time.Now().UTC(),
		Kind:       kind,
		Reason:     reason,
		Detail:     detail,
	}

	if deadLetter.audit != nil {
		deadLetter.audit.TryEnqueueFrame(frame)
	}
}
