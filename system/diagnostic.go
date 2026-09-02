package system

import (
	"sync/atomic"
	"time"

	"github.com/theapemachine/symm/types"
)

// diagnosticGapEMAShift sets the EMA smoothing window for a stage's
// inter-arrival gap: avg += (gap-avg) >> shift, i.e. a half-life of about
// 2^shift samples. 4 settles in ~16 samples without being so short that a
// single slow envelope swings the average.
const diagnosticGapEMAShift = 4

/*
Diagnostic stamps its own label onto every envelope it sees, appending to
Envelope.Boundaries, along with a running summary of how often and how
regularly this exact stage runs. One Diagnostic instance sits at each stage
boundary a Workload declares and is called repeatedly over the process
lifetime, so it keeps its own tiny lock-free counters (count, last-seen time,
EMA gap) rather than recomputing them from scratch per envelope. A consumer
derives topology from consecutive labels, per-hop latency from their AtNs
delta, and per-stage rate/health from the stamped counters — nothing about
the pipeline shape is hand-maintained.
*/
type Diagnostic struct {
	label string

	// group/stage are the ring this stage belongs to and its handler-group
	// index within it, written once by runtime.Workload while it composes its
	// stages (see Compose) — before the ring is admitted, so before any
	// envelope can reach a Step. They are plain fields rather than atomics
	// for exactly that reason: the write happens-before the goroutines that
	// read them are ever started.
	group string
	stage int32

	seqCount atomic.Int64
	lastAtNs atomic.Int64
	avgGapNs atomic.Int64
}

func NewDiagnostic(label string) *Diagnostic {
	return &Diagnostic{label: label}
}

/*
Compose implements runtime.Composed: the Workload that owns this stage tells
it which ring it runs in and which handler group it sits in. Without this the
topology can only be inferred from the order labels appear in a trace, which
is wrong for a concurrent stage — its nodes run side by side against the same
envelope and stamp in whatever order their goroutines finish, so consecutive
labels there describe a race, not a hop.
*/
func (diagnostic *Diagnostic) Compose(group string, stage int) {
	diagnostic.group = group
	diagnostic.stage = int32(stage)
}

func (diagnostic *Diagnostic) Step(envelope *types.Envelope) *types.Envelope {
	return diagnostic.stamp(envelope, 0)
}

/*
StepBacklog implements runtime.BacklogStepper: the Workload hands back how
many sequence numbers behind its producer this envelope already was when it
reached this stage — real ring pressure, not a guess from rates.
*/
func (diagnostic *Diagnostic) StepBacklog(envelope *types.Envelope, backlog int64) *types.Envelope {
	return diagnostic.stamp(envelope, backlog)
}

/*
tracedNode is the interface Traced needs from the node it wraps — exactly
runtime.Node[*types.Envelope], restated here so this package does not import
nomagique/runtime (which already imports system, so importing it back would
cycle).
*/
type tracedNode interface {
	Step(*types.Envelope) *types.Envelope
	Error() error
}

/*
Traced wraps one node from a concurrent stage (a HandlerGroup running several
signals side by side, each on its own goroutine against the same envelope)
with its own labeled Diagnostic, so a signal fan-out can be split into one
topology node per signal instead of collapsing behind a single stage-trailing
Diagnostic. The wrapped node's own Step runs first, then the label is
stamped — same call, no extra stage, so it costs nothing beyond what a
trailing Diagnostic already cost.
*/
type Traced struct {
	tracedNode
	diagnostic *Diagnostic
}

func NewTraced(label string, node tracedNode) *Traced {
	return &Traced{tracedNode: node, diagnostic: NewDiagnostic(label)}
}

/* Compose forwards the owning ring's identity to the wrapped label. */
func (traced *Traced) Compose(group string, stage int) {
	traced.diagnostic.Compose(group, stage)
}

func (traced *Traced) Step(envelope *types.Envelope) *types.Envelope {
	envelope = traced.tracedNode.Step(envelope)

	if traced.Error() != nil || envelope == nil {
		return envelope
	}

	return traced.diagnostic.Step(envelope)
}

func (traced *Traced) StepBacklog(envelope *types.Envelope, backlog int64) *types.Envelope {
	envelope = traced.tracedNode.Step(envelope)

	if traced.Error() != nil || envelope == nil {
		return envelope
	}

	return traced.diagnostic.StepBacklog(envelope, backlog)
}

func (diagnostic *Diagnostic) stamp(envelope *types.Envelope, backlog int64) *types.Envelope {
	now := time.Now().UnixNano()
	count := diagnostic.seqCount.Add(1)
	previous := diagnostic.lastAtNs.Swap(now)

	var lastGapNs int64

	if previous != 0 {
		lastGapNs = now - previous

		avg := diagnostic.avgGapNs.Load()

		if avg == 0 {
			diagnostic.avgGapNs.Store(lastGapNs)
		} else {
			diagnostic.avgGapNs.Store(avg + (lastGapNs-avg)>>diagnosticGapEMAShift)
		}
	}

	envelope.AppendBoundary(types.BoundaryStamp{
		Label:     diagnostic.label,
		AtNs:      now,
		SeqCount:  count,
		AvgGapNs:  diagnostic.avgGapNs.Load(),
		LastGapNs: lastGapNs,
		Backlog:   backlog,
		Group:     diagnostic.group,
		Stage:     diagnostic.stage,
	})

	return envelope
}
