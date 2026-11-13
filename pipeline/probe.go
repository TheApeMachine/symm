package pipeline

import (
	"sync/atomic"
	"time"

	nmruntime "github.com/theapemachine/nomagique/runtime"
)

/*
Probe times one workspace node without changing its contract. Stats are
lock-free so the disruptor path only pays for a clock read and a few atomics.
*/
type Probe struct {
	node  nmruntime.Node
	group string

	n      atomic.Uint64
	lastNs atomic.Int64
	emaNs  atomic.Int64
	seenAt atomic.Int64
}

/*
NewProbe wraps node for the named ring group.
*/
func NewProbe(group string, node nmruntime.Node) *Probe {
	return &Probe{
		node:  node,
		group: group,
	}
}

/*
Step measures node work for one envelope.
*/
func (probe *Probe) Step(envelope *nmruntime.Envelope) *nmruntime.Envelope {
	start := time.Now()
	out := probe.node.Step(envelope)
	probe.record(time.Since(start))

	return out
}

func (probe *Probe) Status() nmruntime.Stage {
	return probe.node.Status()
}

func (probe *Probe) Transition(stage nmruntime.Stage) {
	probe.node.Transition(stage)
}

func (probe *Probe) Error(errs ...error) error {
	return probe.node.Error(errs...)
}

func (probe *Probe) Name() string {
	return probe.node.Name()
}

/*
Group is the disruptor ring this probe sits in.
*/
func (probe *Probe) Group() string {
	return probe.group
}

/*
record updates sample count, last duration, and a 1/8 EMA of work time,
then pulses the dashboard so downstream edges can burst on real Step work.
*/
func (probe *Probe) record(d time.Duration) {
	ns := d.Nanoseconds()
	if ns < 0 {
		ns = 0
	}

	n := probe.n.Add(1)
	probe.lastNs.Store(ns)
	probe.seenAt.Store(time.Now().UnixNano())

	prev := probe.emaNs.Load()
	if prev == 0 {
		probe.emaNs.Store(ns)
	} else {
		probe.emaNs.Store(prev - prev/8 + ns/8)
	}

	Fire(probe.Name(), map[string]any{
		"n":       n,
		"last_ns": ns,
		"ema_ns":  probe.emaNs.Load(),
	})
}

/*
Snapshot is one node row for the dashboard graph.
*/
func (probe *Probe) Snapshot() map[string]any {
	return map[string]any{
		"id":      probe.Name(),
		"group":   probe.group,
		"n":       probe.n.Load(),
		"last_ns": probe.lastNs.Load(),
		"ema_ns":  probe.emaNs.Load(),
		"seen_at": probe.seenAt.Load(),
		"status":  probe.Status().String(),
	}
}
