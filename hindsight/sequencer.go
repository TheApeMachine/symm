package hindsight

import (
	"sync"

	"github.com/theapemachine/errnie"
)

/*
Sequencer hands out stable CaptureIdentity values for a single Run before
parsing happens (§4, §6, §11). It owns the run-local monotonic capture order
and the per-stream epoch/sequence bookkeeping, so an identity is assigned the
moment an external input is observed — never derived later from a parsed event,
a venue timestamp, or a persistence row id.

It is safe for concurrent use: raw inputs arrive on more than one stream, and
each arrival needs a distinct identity.
*/
type Sequencer struct {
	run     RunID
	mu      sync.Mutex
	next    CaptureSequence
	streams map[Stream]streamState
}

/*
streamState is the per-stream bookkeeping a Sequencer maintains: the current
epoch (bumped on every reconnect) and the sequence within that epoch.
*/
type streamState struct {
	epoch    StreamEpoch
	sequence uint64
}

/*
NewSequencer builds a Sequencer assigning identities belonging to the given
Run. A blank Run is a validation error: the identity would be ambiguous.
*/
func NewSequencer(run RunID) (*Sequencer, error) {
	if run == "" {
		return nil, errnie.Error(errnie.Err(
			errnie.Validation,
			"hindsight: sequencer requires a run identity",
			nil,
		))
	}

	return &Sequencer{
		run:     run,
		streams: make(map[Stream]streamState),
	}, nil
}

/*
Assign mints the next CaptureIdentity for one observed external input on the
given stream, incrementing the run-local capture order and the stream sequence.
It is called before the input is parsed.
*/
func (sequencer *Sequencer) Assign(stream Stream) (CaptureIdentity, error) {
	sequencer.mu.Lock()
	defer sequencer.mu.Unlock()

	state, ok := sequencer.streams[stream]

	if !ok {
		state = streamState{epoch: 1, sequence: 0}
	}

	sequencer.next++
	state.sequence++

	sequencer.streams[stream] = state

	return CaptureIdentity{
		Run:            sequencer.run,
		Sequence:       sequencer.next,
		Stream:         stream,
		StreamEpoch:    state.epoch,
		StreamSequence: state.sequence,
	}, nil
}

/*
Reconnect starts a new epoch on the given stream. The StreamSequence resets to
zero so the next frame begins a fresh sequence in its new epoch, while the
run-local capture order keeps increasing (§7).
*/
func (sequencer *Sequencer) Reconnect(stream Stream) {
	sequencer.mu.Lock()
	defer sequencer.mu.Unlock()

	state := sequencer.streams[stream]
	state.epoch++
	state.sequence = 0
	sequencer.streams[stream] = state
}

/*
Latest returns the capture sequence most recently assigned in this run: the
frame the system had observed up to at the moment of the call.

It exists for facts that happen between frames rather than on one. A position
exit is the case: the desk's Stoploss acts on the market it has seen, but it
commits no decision of its own, so the exit has no envelope of its own to be
correlated to. Stamping the latest assigned sequence records the tape position
the exit was taken at, which is the honest answer to "where on the tape did
this happen" — it is not a claim that the exit was caused by that frame.

Zero before the first frame of a run has been assigned.
*/
func (sequencer *Sequencer) Latest() CaptureSequence {
	sequencer.mu.Lock()
	defer sequencer.mu.Unlock()

	return sequencer.next
}
