package data

import "sync"

/*
Span is one measured stretch of a tick.

WorkNs is time spent inside the stage. WaitNs is time the stage sat blocked
before that work began: the disruptor queue, or the barrier of the previous
stage. Side marks a measurement that shares the machine but is not on this
tick's critical path (the book apply runs on its own ingress).

None of this is written into the stored value.
*/
type Span struct {
	Name    string
	Group   string
	Stage   int
	WaitNs  int64
	WorkNs  int64
	Backlog int64
	Side    bool
}

type diag struct {
	mu       sync.Mutex
	spans    []Span
	enqueued int64
	done     [8]atomicInt64
}

// atomicInt64 is a local alias so diag can live beside the standard library
// atomic types without the Value header importing a name clash. The methods
// are the sync/atomic Int64 operations, inlined below as a plain int64 with
// the diag mutex covering the stage-done cells too: a tick has a handful of
// stages and the mutex is already taken to append a span.
type atomicInt64 = struct{ v int64 }

func (value *Value) loadDiag() *diag {
	if value == nil {
		return nil
	}

	if existing := value.trace.Load(); existing != nil {
		return existing
	}

	next := &diag{}

	if value.trace.CompareAndSwap(nil, next) {
		return next
	}

	return value.trace.Load()
}

/*
MarkEnqueued records the moment the tick was committed to the workspace.
Later stages measure their queue wait from here.
*/
func (value *Value) MarkEnqueued(atNs int64) {
	if diag := value.loadDiag(); diag != nil {
		diag.mu.Lock()
		diag.enqueued = atNs
		diag.mu.Unlock()
	}
}

/*
Enqueued is the MarkEnqueued instant, or 0 when the tick was never committed.
*/
func (value *Value) Enqueued() int64 {
	diag := value.loadDiag()

	if diag == nil {
		return 0
	}

	diag.mu.Lock()
	defer diag.mu.Unlock()

	return diag.enqueued
}

/*
MarkStageDone records when a stage finished. Concurrent siblings in one
handler group all call it; the time that sticks is the latest, which is the
barrier the next stage actually waited on.
*/
func (value *Value) MarkStageDone(stage int, atNs int64) {
	diag := value.loadDiag()

	if diag == nil || stage < 0 || stage >= len(diag.done) {
		return
	}

	diag.mu.Lock()
	defer diag.mu.Unlock()

	if atNs > diag.done[stage].v {
		diag.done[stage].v = atNs
	}
}

/*
StageDone is the latest completion time recorded for stage, or 0.
*/
func (value *Value) StageDone(stage int) int64 {
	diag := value.loadDiag()

	if diag == nil || stage < 0 || stage >= len(diag.done) {
		return 0
	}

	diag.mu.Lock()
	defer diag.mu.Unlock()

	return diag.done[stage].v
}

/*
Note appends one span. Handlers of the same tick call it concurrently.
*/
func (value *Value) Note(span Span) {
	diag := value.loadDiag()

	if diag == nil {
		return
	}

	diag.mu.Lock()
	diag.spans = append(diag.spans, span)
	diag.mu.Unlock()
}

/*
Spans returns a copy of the spans noted so far.
*/
func (value *Value) Spans() []Span {
	diag := value.loadDiag()

	if diag == nil {
		return nil
	}

	diag.mu.Lock()
	defer diag.mu.Unlock()

	out := make([]Span, len(diag.spans))
	copy(out, diag.spans)

	return out
}
