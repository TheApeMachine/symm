package logic

import (
	"context"
	"sync"
	"time"

	"github.com/theapemachine/datura/dmt"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/audit"
	"github.com/theapemachine/symm/types"
)

/*
remSleep accumulates episodic observation windows and runs DMT REM
consolidation off the analyzer hot path. Ambiguity still gates the request;
single-flight prevents a second REM from stacking while one is in flight.
*/
type remSleep struct {
	ctx            context.Context
	cancel         context.CancelFunc
	tree           *dmt.Tree
	recorder       *audit.Recorder
	mu             sync.Mutex
	busy           bool
	rerunRequested bool
	rerunTick      int64
	finished       chan struct{}
	from           time.Time
	through        time.Time
	pending        int
	lastFrom       time.Time
	lastThrough    time.Time
	lastReplays    int
}

/*
newREMSleep wires REM onto an optional cognitive tree. A nil tree keeps the
scheduler inert so tests without DMT still exercise analyzer Update.
*/
func newREMSleep(ctx context.Context, tree *dmt.Tree) *remSleep {
	if ctx == nil {
		ctx = context.Background()
	}

	ctx, cancel := context.WithCancel(ctx)

	return &remSleep{
		ctx:    ctx,
		cancel: cancel,
		tree:   tree,
	}
}

/*
SetRecorder attaches the runtime audit stream used for rem_begin, rem, and
rem_deferred breadcrumbs.
*/
func (rem *remSleep) SetRecorder(recorder *audit.Recorder) {
	if rem == nil {
		return
	}

	rem.recorder = recorder
}

/*
Pending reports how many cognized observations wait for the next REM window.
*/
func (rem *remSleep) Pending() int {
	if rem == nil {
		return 0
	}

	rem.mu.Lock()
	defer rem.mu.Unlock()

	return rem.pending
}

/*
Accumulate widens the pending REM interval with cognized observation times.
*/
func (rem *remSleep) Accumulate(observations []time.Time) {
	if rem == nil {
		return
	}

	rem.mu.Lock()
	defer rem.mu.Unlock()

	if rem.ctx.Err() != nil {
		return
	}

	for _, at := range observations {
		if rem.from.IsZero() || at.Before(rem.from) {
			rem.from = at
		}

		if rem.through.IsZero() || at.After(rem.through) {
			rem.through = at
		}

		rem.pending++
	}
}

/*
Request starts asynchronous REM when the ambiguity gate fires and work is
pending. If REM is already running, observations stay queued, rerun is marked,
and rem_deferred is audited so the tick path never blocks on WAL decay.
*/
func (rem *remSleep) Request(tick int64) {
	if rem == nil || rem.tree == nil {
		return
	}

	rem.mu.Lock()

	if rem.ctx.Err() != nil {
		rem.mu.Unlock()
		return
	}

	if rem.pending == 0 {
		rem.mu.Unlock()
		return
	}

	if rem.busy {
		rem.rerunRequested = true
		rem.rerunTick = tick
		errnie.Error(audit.Phase(rem.recorder, tick, "rem_deferred", map[string]any{
			"pending": rem.pending,
			"from":    rem.from.UnixNano(),
			"through": rem.through.UnixNano(),
		}))
		rem.mu.Unlock()
		return
	}

	rem.launchPass(tick)
}

/*
launchPass drains the pending REM window and starts one consolidation pass.
The caller must hold rem.mu; launchPass releases it before returning.
*/
func (rem *remSleep) launchPass(tick int64) {
	from := rem.from
	through := rem.through
	pending := rem.pending
	rem.from = time.Time{}
	rem.through = time.Time{}
	rem.pending = 0
	rem.busy = true
	rem.rerunRequested = false
	rem.rerunTick = 0
	finished := make(chan struct{})
	rem.finished = finished
	rem.mu.Unlock()

	go rem.execute(tick, from, through, pending, finished)
}

/*
execute owns one REM consolidation pass and publishes completion state for the
next cognition stamp.
*/
func (rem *remSleep) execute(
	tick int64,
	from time.Time,
	through time.Time,
	pending int,
	finished chan struct{},
) {
	defer close(finished)

	remStarted := time.Now()

	errnie.Error(audit.Phase(rem.recorder, tick, "rem_begin", map[string]any{
		"pending": pending,
		"from":    from.UnixNano(),
		"through": through.UnixNano(),
	}))

	err := rem.tree.ExecuteREMSleepConsolidation(
		uint64(from.UnixNano()),
		uint64(through.UnixNano()),
	)

	if err != nil {
		errnie.Error(errnie.Err(
			errnie.IO,
			"logic rem: consolidation failed",
			err,
		))

		rem.mu.Lock()

		if rem.ctx.Err() == nil {
			if rem.from.IsZero() || from.Before(rem.from) {
				rem.from = from
			}

			if rem.through.IsZero() || through.After(rem.through) {
				rem.through = through
			}

			rem.pending += pending
		}

		rem.busy = false
		rem.rerunRequested = false
		rem.rerunTick = 0
		rem.mu.Unlock()

		return
	}

	errnie.Error(audit.Phase(rem.recorder, tick, "rem", map[string]any{
		"pending": pending,
		"ns":      time.Since(remStarted).Nanoseconds(),
	}))

	rem.mu.Lock()
	rem.lastFrom = from
	rem.lastThrough = through
	rem.lastReplays = pending
	rem.busy = false

	if rem.ctx.Err() != nil || rem.pending == 0 {
		rem.mu.Unlock()
		return
	}

	nextTick := rem.rerunTick

	if nextTick == 0 {
		nextTick = tick
	}

	rem.launchPass(nextTick)
}

/*
Stamp copies the latest completed REM window onto every cognition reading so
downstream consumers can see consolidation progress without waiting on the
background worker.
*/
func (rem *remSleep) Stamp(thesis *types.Thesis) {
	if rem == nil || thesis == nil {
		return
	}

	rem.mu.Lock()
	lastFrom := rem.lastFrom
	lastThrough := rem.lastThrough
	lastReplays := rem.lastReplays
	rem.mu.Unlock()

	thesis.Cognition.Range(func(key, value any) bool {
		reading := value.(types.Cognition)
		reading.REMFrom = lastFrom
		reading.REMThrough = lastThrough
		reading.REMReplays = lastReplays
		thesis.Cognition.Store(key, reading)

		return true
	})
}

/*
Await blocks until an in-flight REM finishes so the next foreground cognition
pass cannot read or train the same tree during consolidation.
*/
func (rem *remSleep) Await() {
	if rem == nil {
		return
	}

	for {
		rem.mu.Lock()
		busy := rem.busy
		finished := rem.finished
		rem.mu.Unlock()

		if !busy || finished == nil {
			return
		}

		select {
		case <-finished:
		case <-rem.ctx.Done():
			return
		}
	}
}

/*
Close cancels future REM scheduling, discards queued shutdown work, and joins
the current consolidation before the cognitive tree can be closed.
*/
func (rem *remSleep) Close() {
	if rem == nil {
		return
	}

	if rem.cancel != nil {
		rem.cancel()
	}

	rem.mu.Lock()
	rem.pending = 0
	rem.from = time.Time{}
	rem.through = time.Time{}
	rem.rerunRequested = false
	rem.rerunTick = 0
	finished := rem.finished
	rem.mu.Unlock()

	if finished != nil {
		<-finished
	}
}
