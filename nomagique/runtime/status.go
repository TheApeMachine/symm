package runtime

import (
	"slices"
	"sync/atomic"

	"github.com/theapemachine/errnie"
)

type Stage uint8

const (
	INIT Stage = iota
	OK
	ERROR
	FATAL
	READY
	BUSY
	WAITING
	DONE
)

/*
String converts the state to its string representation.
*/
func (stage Stage) String() string {
	switch stage {
	case INIT:
		return "init"
	case OK:
		return "ok"
	case ERROR:
		return "error"
	case FATAL:
		return "fatal"
	case READY:
		return "ready"
	case BUSY:
		return "busy"
	case WAITING:
		return "waiting"
	case DONE:
		return "done"
	}

	return "unknown"
}

/*
transitions determines the legal state transitions.
*/
var transitions = map[Stage][]Stage{
	INIT:    {ERROR, FATAL, BUSY, WAITING, READY},
	OK:      {ERROR, FATAL, DONE},
	ERROR:   {FATAL, OK, INIT, READY},
	FATAL:   {INIT},
	READY:   {ERROR, FATAL, BUSY, WAITING, DONE},
	BUSY:    {ERROR, FATAL, READY, WAITING, DONE},
	WAITING: {ERROR, FATAL, BUSY, DONE, READY},
	DONE:    {ERROR, FATAL, READY},
}

/*
StatusTracker is a general indicator of lifecycle stages.
*/
type StatusTracker struct {
	err     error
	current atomic.Value
}

/*
NewStatus intializes a new status management object.
*/
func NewStatus() *StatusTracker {
	tracker := &StatusTracker{}
	tracker.current.Store(INIT)
	return tracker
}

/*
Transition the state into one of the legal follow-up states,
determined by the transition mapping. Thread-safe.
*/
func (tracker *StatusTracker) Transition(stage Stage) *StatusTracker {
	for {
		current, ok := tracker.current.Load().(Stage)

		if !ok {
			return tracker
		}

		// Re-entering the current stage is a benign no-op, not an error: every
		// hot path that reports a same-stage condition (a repeated subscription
		// rejection, a repeated ERROR) calls Transition with the state already
		// held. Treating that as illegal wedges callers and floods the log.
		if current == stage {
			return tracker
		}

		valid := slices.Contains(transitions[current], stage)

		if !valid {
			tracker.err = errnie.Error(errnie.Err(
				errnie.NotAcceptable,
				"status: illegal transition",
				nil,
			))

			return tracker
		}

		// Atomically swap only if another goroutine hasn't modified it in the meantime
		if tracker.current.CompareAndSwap(current, stage) {
			return tracker
		}
	}
}

/*
Current status.
*/
func (tracker *StatusTracker) Current() Stage {
	return tracker.current.Load().(Stage)
}
