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
	READY:   {ERROR, FATAL, BUSY, WAITING},
	BUSY:    {ERROR, FATAL, WAITING, DONE},
	WAITING: {ERROR, FATAL, BUSY, DONE, READY},
	DONE:    {ERROR, FATAL, READY},
}

/*
Status is a general indicator of lifecycle stages.
*/
type Status struct {
	err     error
	current atomic.Value
}

/*
NewStatus intializes a new status management object.
*/
func NewStatus() *Status {
	status := &Status{}
	status.current.Store(INIT)
	return status
}

/*
Transition the state into one of the legal follow-up states,
determined by the transition mapping. Thread-safe.
*/
func (status *Status) Transition(stage Stage) *Status {
	for {
		current, ok := status.current.Load().(Stage)

		if !ok {
			return status
		}

		valid := slices.Contains(transitions[current], stage)

		if !valid {
			status.err = errnie.Error(errnie.Err(
				errnie.NotAcceptable,
				"status: illegal transition",
				nil,
			))

			return status
		}

		// Atomically swap only if another goroutine hasn't modified it in the meantime
		if status.current.CompareAndSwap(current, stage) {
			return status
		}
	}
}

/*
Current status.
*/
func (status *Status) Current() Stage {
	return status.current.Load().(Stage)
}
