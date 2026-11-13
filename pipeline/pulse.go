package pipeline

import (
	"sync"
	"sync/atomic"
	"time"

	"github.com/theapemachine/symm/ui"
)

/*
flushEvery merges pending measurements into one hub frame so venue + probe
pulses stay live without saturating the websocket queue.
*/
const flushEvery = 16 * time.Millisecond

/*
Pulse is a named activity meter outside the disruptor Node contract — venue
reads, child stages inside training/model, and the S3 queue.
*/
type Pulse struct {
	id     string
	parent string

	n      atomic.Uint64
	lastNs atomic.Int64
	emaNs  atomic.Int64
	seenAt atomic.Int64
	queue  atomic.Int64
	gauge  atomic.Bool
}

var pulses sync.Map

var (
	pendingMu     sync.Mutex
	pending       = map[string]map[string]any{}
	flushScheduled atomic.Bool
)

/*
Define registers a pulse under an optional parent compound id.
*/
func Define(id, parent string) *Pulse {
	if id == "" {
		return nil
	}

	if loaded, ok := pulses.Load(id); ok {
		pulse := loaded.(*Pulse)
		if parent != "" {
			pulse.parent = parent
		}
		return pulse
	}

	pulse := &Pulse{id: id, parent: parent}
	actual, _ := pulses.LoadOrStore(id, pulse)
	return actual.(*Pulse)
}

/*
Hit marks activity and queues a live dashboard pulse.
*/
func Hit(id string) {
	pulse := Define(id, "")
	if pulse == nil {
		return
	}

	pulse.n.Add(1)
	pulse.seenAt.Store(time.Now().UnixNano())
	pulse.queueLive(nil)
}

/*
Time records one timed sample and queues a live dashboard pulse.
*/
func Time(id string, d time.Duration) {
	pulse := Define(id, "")
	if pulse == nil {
		return
	}

	ns := d.Nanoseconds()
	if ns < 0 {
		ns = 0
	}

	n := pulse.n.Add(1)
	pulse.lastNs.Store(ns)
	pulse.seenAt.Store(time.Now().UnixNano())

	prev := pulse.emaNs.Load()
	if prev == 0 {
		pulse.emaNs.Store(ns)
	} else {
		pulse.emaNs.Store(prev - prev/8 + ns/8)
	}

	pulse.queueLive(map[string]any{
		"n":       n,
		"last_ns": ns,
		"ema_ns":  pulse.emaNs.Load(),
	})
}

/*
Queue sets a gauge on a pulse (S3 backlog depth) without a live fan-out —
use Wrote when an enqueue should burst the dashboard.
*/
func Queue(id string, depth int64) {
	pulse := Define(id, "")
	if pulse == nil {
		return
	}

	if depth < 0 {
		depth = 0
	}

	pulse.queue.Store(depth)
	pulse.gauge.Store(true)
	pulse.seenAt.Store(time.Now().UnixNano())
}

/*
Wrote marks an S3 enqueue: lights s3 and the queue from the post-enqueue depth.
*/
func Wrote(depth int64) {
	if depth < 0 {
		depth = 0
	}

	Hit("s3")

	pulse := Define("queue", "store")
	pulse.queue.Store(depth)
	pulse.gauge.Store(true)
	pulse.seenAt.Store(time.Now().UnixNano())
	pulse.n.Add(1)
	pulse.queueLive(map[string]any{"queue": depth})
}

/*
Fire publishes one live diag pulse for a named stage (ingress timing, venue).
*/
func Fire(id string, extra map[string]any) {
	pulse := Define(id, "")
	if pulse == nil {
		return
	}

	pulse.queueLive(extra)
}

/*
queueLive merges this stage into the next flushed diag_pulses frame.
*/
func (pulse *Pulse) queueLive(extra map[string]any) {
	now := time.Now().UnixNano()
	frame := map[string]any{
		"id": pulse.id,
		"at": now,
		"n":  pulse.n.Load(),
	}

	if last := pulse.lastNs.Load(); last > 0 {
		frame["last_ns"] = last
	}

	if ema := pulse.emaNs.Load(); ema > 0 {
		frame["ema_ns"] = ema
	}

	if pulse.gauge.Load() {
		frame["queue"] = pulse.queue.Load()
	}

	for key, value := range extra {
		frame[key] = value
	}

	pendingMu.Lock()
	pending[pulse.id] = frame
	pendingMu.Unlock()

	if !flushScheduled.CompareAndSwap(false, true) {
		return
	}

	go func() {
		time.Sleep(flushEvery)
		flushLive()
	}()
}

func flushLive() {
	flushScheduled.Store(false)

	if ui.UI == nil || !ui.UI.Active() {
		pendingMu.Lock()
		clear(pending)
		pendingMu.Unlock()
		return
	}

	pendingMu.Lock()
	if len(pending) == 0 {
		pendingMu.Unlock()
		return
	}

	batch := make([]map[string]any, 0, len(pending))
	for _, row := range pending {
		batch = append(batch, row)
	}
	clear(pending)
	pendingMu.Unlock()

	ui.UI.WriteJSON(map[string]any{
		"diag_pulses": batch,
	})
}

func (pulse *Pulse) snapshot() map[string]any {
	row := map[string]any{
		"id":      pulse.id,
		"n":       pulse.n.Load(),
		"last_ns": pulse.lastNs.Load(),
		"ema_ns":  pulse.emaNs.Load(),
		"seen_at": pulse.seenAt.Load(),
		"status":  "ok",
	}

	if pulse.parent != "" {
		row["parent"] = pulse.parent
	}

	if pulse.gauge.Load() {
		row["queue"] = pulse.queue.Load()
	}

	return row
}

func pulseSnapshots() []map[string]any {
	out := make([]map[string]any, 0, 16)

	pulses.Range(func(_, value any) bool {
		out = append(out, value.(*Pulse).snapshot())
		return true
	})

	return out
}

/*
pulseSkeletons lists registered pulse ids for topology without measurement fields.
*/
func pulseSkeletons() []map[string]any {
	out := make([]map[string]any, 0, 16)

	pulses.Range(func(_, value any) bool {
		pulse := value.(*Pulse)
		row := map[string]any{
			"id":     pulse.id,
			"status": "ok",
		}
		if pulse.parent != "" {
			row["parent"] = pulse.parent
		}
		out = append(out, row)
		return true
	})

	return out
}
