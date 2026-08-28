package ui

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/logic/graph"
	"github.com/theapemachine/symm/logic/manifold"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/runtime"
	"github.com/theapemachine/symm/telemetry"
	wire "github.com/theapemachine/symm/telemetry/generated/telemetry"
	"github.com/theapemachine/symm/types"
)

/*
stageClock is the two accumulating answers a pipeline node ever needs to report:
how many Steps it has run and their combined wall time, so "average step time"
is one division the frontend does from timestamped counters. Nothing is
snapshotted, cloned, or replayed — these are the live atomics, read in place.
*/
type stageClock struct {
	count    atomic.Uint64
	totalNs  atomic.Uint64
	lastNs   atomic.Uint64
	maxNs    atomic.Uint64
	lastAtNs atomic.Int64
}

/*
ClockSnapshot is one node's live read: count and summed nanoseconds.
*/
type ClockSnapshot struct {
	Name     string `json:"name"`
	Count    uint64 `json:"count"`
	TotalNs  uint64 `json:"total_ns"`
	LastNs   uint64 `json:"last_ns"`
	MaxNs    uint64 `json:"max_ns"`
	LastAtNs int64  `json:"last_at_ns"`
}

/*
HopSnapshot is an unused compatibility slot retained so the wire schema and the
frontend keep compiling; the stream model derives edge timing from node clocks.
*/
type HopSnapshot struct {
	From    string `json:"from"`
	To      string `json:"to"`
	Count   uint64 `json:"count"`
	TotalNs uint64 `json:"total_ns"`
	LastNs  uint64 `json:"last_ns"`
	MaxNs   uint64 `json:"max_ns"`
}

/*
QueueSnapshot is one bounded ring's live read: current occupancy and drops.
*/
type QueueSnapshot struct {
	Name      string   `json:"name"`
	Kind      string   `json:"kind"`
	Writers   []string `json:"writers"`
	Readers   []string `json:"readers"`
	Depth     uint64   `json:"depth"`
	Cap       uint64   `json:"cap"`
	HighWater uint64   `json:"high_water"`
	Symbols   uint64   `json:"symbols"`
	Dropped   uint64   `json:"dropped"`
}

type ErrorSnapshot struct {
	Source  string `json:"source"`
	Message string `json:"message"`
	Caller  string `json:"caller"`
	AtNs    int64  `json:"at_ns"`
}

type PassStatus struct {
	State       string `json:"state"`
	InFlightNs  int64  `json:"in_flight_ns"`
	LastPassNs  int64  `json:"last_pass_ns"`
	SinceLastNs int64  `json:"since_last_ns"`
}

type GoroutineOwner struct {
	Owner string `json:"owner"`
	Count int64  `json:"count"`
	State string `json:"state"`
}

/*
StreamDiagnostics is the thin wire record: the two facts (node step time, ring
depth) across every live node and ring, plus the toggle state.
*/
type StreamDiagnostics struct {
	Status     string            `json:"status"`
	Enabled    bool              `json:"enabled"`
	AtNs       int64             `json:"at_ns"`
	StartedNs  int64             `json:"started_ns"`
	Stages     []ClockSnapshot   `json:"stages"`
	Queues     []QueueSnapshot   `json:"queues"`
	Pass       PassStatus        `json:"pass"`
	Hops       []HopSnapshot     `json:"hops"`
	Errors     []ErrorSnapshot   `json:"errors"`
	Goroutines []GoroutineOwner  `json:"goroutines"`
}

/*
queueTopology is the static fan-out of one pipeline buffer: who writes it, who
reads it, and a closure that reads its aggregate ring telemetry by the
concrete Go type flowing through it (there is no topic string to key on —
routing is by type, so diagnostics reads telemetry the same way).
*/
type queueTopology struct {
	name     string
	kind     string
	writers  []string
	readers  []string
	snapshot func(bus *runtime.Workspace) runtime.Snapshot
}

/*
queueTopologies is the fixed analytical data plane the diagnostics page renders.
The frontend places these exact ids on its wiring graph, so the topology is a
stable contract, not a discovery result.
*/
var queueTopologies = []queueTopology{
	{
		name: "ingress.tickers", kind: "ingress",
		writers: []string{"crypto"},
		readers: []string{"correlation", "leadlag", "liquidity", "pumpdump", "sentiment", "resonance", "desk"},
		snapshot: func(bus *runtime.Workspace) runtime.Snapshot { return runtime.TypeSnapshot[kraken.TickerData](bus) },
	},
	{
		name: "ingress.trades", kind: "ingress",
		writers: []string{"crypto"},
		readers: []string{"cvd", "derivatives", "hawkes", "pumpdump", "toxicity"},
		snapshot: func(bus *runtime.Workspace) runtime.Snapshot { return runtime.TypeSnapshot[kraken.TradeData](bus) },
	},
	{
		name: "ingress.level3", kind: "ingress",
		writers: []string{"crypto"},
		readers: []string{"depthflow"},
		snapshot: func(bus *runtime.Workspace) runtime.Snapshot { return runtime.TypeSnapshot[kraken.Level3Data](bus) },
	},
	{
		name: "measurements", kind: "rail",
		writers: []string{"correlation", "cvd", "depthflow", "derivatives", "hawkes", "leadlag", "liquidity", "pumpdump", "sentiment", "toxicity"},
		readers: []string{"category", "manifold", "graph"},
		snapshot: func(bus *runtime.Workspace) runtime.Snapshot { return runtime.TypeSnapshot[*data.Measurement[float64]](bus) },
	},
	{
		name: "derived.category", kind: "derived",
		writers: []string{"category"},
		readers: []string{"cognition", "graph"},
		snapshot: func(bus *runtime.Workspace) runtime.Snapshot { return runtime.TypeSnapshot[[]types.Category](bus) },
	},
	{
		name: "derived.causal", kind: "derived",
		writers: []string{"causal"},
		readers: []string{"graph"},
		snapshot: func(bus *runtime.Workspace) runtime.Snapshot { return runtime.TypeSnapshot[*types.CausalOutput](bus) },
	},
	{
		name: "derived.cognition", kind: "derived",
		writers: []string{"cognition"},
		readers: []string{"graph"},
		snapshot: func(bus *runtime.Workspace) runtime.Snapshot { return runtime.TypeSnapshot[*types.Cognition](bus) },
	},
	{
		name: "derived.graph", kind: "derived",
		writers: []string{"graph"},
		readers: []string{"planner"},
		snapshot: func(bus *runtime.Workspace) runtime.Snapshot { return runtime.TypeSnapshot[*graph.GraphUpdate](bus) },
	},
	{
		name: "derived.resonance", kind: "derived",
		writers: []string{"resonance"},
		readers: []string{"causal", "graph"},
		snapshot: func(bus *runtime.Workspace) runtime.Snapshot { return runtime.TypeSnapshot[*types.ResonanceArtifact](bus) },
	},
	{
		name: "decisions", kind: "strategy",
		writers: []string{"planner"},
		readers: []string{"mcts", "allocation", "desk"},
		snapshot: func(bus *runtime.Workspace) runtime.Snapshot { return runtime.TypeSnapshot[*types.StrategyRound](bus) },
	},
	{
		name: "desk.ticker", kind: "broker",
		writers: []string{"crypto"},
		readers: []string{"desk"},
		snapshot: func(bus *runtime.Workspace) runtime.Snapshot { return runtime.TypeSnapshot[kraken.TickerData](bus) },
	},
	{
		name: "desk.executions", kind: "broker",
		writers: []string{"websocket-api"},
		readers: []string{"desk"},
		snapshot: func(bus *runtime.Workspace) runtime.Snapshot { return runtime.TypeSnapshot[kraken.ExecutionData](bus) },
	},
	{
		name: "positions", kind: "broker",
		writers: []string{"desk"},
		readers: []string{"audit", "hub"},
		snapshot: func(bus *runtime.Workspace) runtime.Snapshot { return runtime.TypeSnapshot[*types.StrategyRound](bus) },
	},
	{
		name: "ui.dashboard", kind: "ui",
		writers: []string{"crypto", "category", "manifold", "causal", "cognition", "graph", "resonance", "planner", "allocation", "desk", "diagnostics"},
		readers: []string{"hub"},
		snapshot: func(bus *runtime.Workspace) runtime.Snapshot { return runtime.TypeSnapshot[*types.UIFrame](bus) },
	},
	{
		name: "ui.manifold", kind: "ui",
		writers: []string{"manifold"},
		readers: []string{"webrtc-hub"},
		snapshot: func(bus *runtime.Workspace) runtime.Snapshot { return runtime.TypeSnapshot[*manifold.State](bus) },
	},
	{
		name: "ui.diagnostics", kind: "ui",
		writers: []string{"diagnostics"},
		readers: []string{"webrtc-hub"},
		snapshot: func(bus *runtime.Workspace) runtime.Snapshot { return runtime.TypeSnapshot[[]byte](bus) },
	},
}

// DiagnosticsControl is implemented by the collector so the hub's runtime
// switch and the /diagnostics endpoints reach it.
var _ DiagnosticsControl = (*Diagnostics)(nil)

/*
Diagnostics collects the two facts. Each node reports its Step time through
ObserveModule; each ring's occupancy is read directly from the workspace's
subscriber atomics on the heartbeat. There are no scanners, no stack walks, no
accumulators beyond the two sums, and no defensive copies.
*/
type Diagnostics struct {
	ctx      context.Context
	cancel   context.CancelFunc
	started  time.Time
	interval time.Duration
	bus      *runtime.Workspace
	feed     *runtime.Feed

	// disabled is set when collection is switched off; the zero value keeps the
	// collector enabled so short-lived call sites and tests preserve the
	// historical always-on behavior without explicit configuration.
	disabled atomic.Bool

	// modules: node name -> *stageClock. Written by ObserveModule on the hot
	// path with atomic adds; read only on the heartbeat.
	modules sync.Map
}

/*
NewDiagnostics constructs the collector. The heartbeat does not start until
Run is invoked, so tests can publish snapshots directly.
*/
func NewDiagnostics(ctx context.Context, bus *runtime.Workspace) *Diagnostics {
	ctx, cancel := context.WithCancel(ctx)

	return &Diagnostics{
		ctx:     ctx,
		cancel:  cancel,
		started: time.Now(),
		bus:     bus,
		feed:    bus.NewFeed(),
	}
}

/*
Run emits one diagnostics snapshot per heartbeat until Close. Publishing is
skipped while collection is switched off, so the disabled state costs nothing.
*/
func (diagnostics *Diagnostics) Run() error {
	interval := diagnostics.interval
	if interval <= 0 {
		interval = 250 * time.Millisecond
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-diagnostics.ctx.Done():
			return nil
		case <-ticker.C:
			if !diagnostics.DiagnosticsEnabled() {
				continue
			}

			diagnostics.publish()
		}
	}
}

/*
Close stops the heartbeat.
*/
func (diagnostics *Diagnostics) Close() error {
	if diagnostics == nil {
		return nil
	}

	diagnostics.cancel()

	return nil
}

/*
SetDiagnosticsEnabled switches collection on or off at runtime. Passing false
stops per-observation timing and drops the heartbeat to an idle cadence,
leaving near-zero overhead on the market data path.
*/
func (diagnostics *Diagnostics) SetDiagnosticsEnabled(enabled bool) {
	if diagnostics == nil {
		return
	}

	diagnostics.disabled.Store(!enabled)
}

/*
DiagnosticsEnabled reports whether the diagnostics collector is switched on.
*/
func (diagnostics *Diagnostics) DiagnosticsEnabled() bool {
	return diagnostics != nil && !diagnostics.disabled.Load()
}

/*
ObserveModule returns the diagnostics module clock hook so any pipeline stage
can report its per-step duration into the same clock bank.
*/
func (diagnostics *Diagnostics) ObserveModule() func(string, time.Duration) {
	if diagnostics == nil {
		return nil
	}

	return diagnostics.applyModule
}

/*
applyModule sums one Step's duration into the node's running total. This is the
entire hot-path cost of diagnostics: a few atomic adds, no allocation.
*/
func (diagnostics *Diagnostics) applyModule(name string, duration time.Duration) {
	if !diagnostics.DiagnosticsEnabled() {
		return
	}

	clock := diagnostics.stage(name)
	nanos := uint64(duration)

	clock.count.Add(1)
	clock.totalNs.Add(nanos)
	clock.lastNs.Store(nanos)
	clock.lastAtNs.Store(time.Now().UnixNano())

	for {
		maxNs := clock.maxNs.Load()
		if nanos <= maxNs || clock.maxNs.CompareAndSwap(maxNs, nanos) {
			return
		}
	}
}

func (diagnostics *Diagnostics) stage(name string) *stageClock {
	if found, ok := diagnostics.modules.Load(name); ok {
		return found.(*stageClock)
	}

	created := &stageClock{}
	actual, _ := diagnostics.modules.LoadOrStore(name, created)

	return actual.(*stageClock)
}

/*
stageSnapshots reads every node's counters live. The result is a fresh slice
because the wire encode needs one; it is the single allocation the heartbeat
pays, not per-event work.
*/
func (diagnostics *Diagnostics) stageSnapshots() []ClockSnapshot {
	out := make([]ClockSnapshot, 0, 32)

	diagnostics.modules.Range(func(key, value any) bool {
		clock := value.(*stageClock)
		out = append(out, ClockSnapshot{
			Name:     key.(string),
			Count:    clock.count.Load(),
			TotalNs:  clock.totalNs.Load(),
			LastNs:   clock.lastNs.Load(),
			MaxNs:    clock.maxNs.Load(),
			LastAtNs: clock.lastAtNs.Load(),
		})

		return true
	})

	return out
}

/*
Snapshot returns the live read for one heartbeat.
*/
func (diagnostics *Diagnostics) Snapshot() StreamDiagnostics {
	stream := StreamDiagnostics{
		Status:     "flowing",
		Enabled:    diagnostics.DiagnosticsEnabled(),
		AtNs:       time.Now().UnixNano(),
		StartedNs:  diagnostics.started.UnixNano(),
		Stages:     diagnostics.stageSnapshots(),
		Queues:     diagnostics.queueSnapshots(),
		Pass:       PassStatus{State: "idle"},
		Hops:       []HopSnapshot{},
		Errors:     []ErrorSnapshot{},
		Goroutines: []GoroutineOwner{},
	}

	if !diagnostics.DiagnosticsEnabled() {
		stream.Status = "disabled"
		stream.Stages = []ClockSnapshot{}
		stream.Queues = []QueueSnapshot{}
	}

	return stream
}

/*
queueSnapshots reads each ring's occupancy straight from the workspace: depth is
published-minus-completed (actual on-ring backlog), never a fabricated count.
Writer/reader wiring is the pipeline's static fan-out.
*/
func (diagnostics *Diagnostics) queueSnapshots() []QueueSnapshot {
	if diagnostics.bus == nil {
		return []QueueSnapshot{}
	}

	queues := make([]QueueSnapshot, 0, len(queueTopologies))

	for _, topology := range queueTopologies {
		snapshot := topology.snapshot(diagnostics.bus)

		queues = append(queues, QueueSnapshot{
			Name:    topology.name,
			Kind:    topology.kind,
			Writers: topology.writers,
			Readers: topology.readers,
			Depth:   snapshot.Pending,
			Cap:     runtime.SubscriberCapacity,
			Symbols: snapshot.Lanes,
			Dropped: snapshot.Dropped,
		})
	}

	return queues
}

/*
publish ships one live read to the two outbound planes: the fluid WebRTC
channel the diagnostics page consumes, and the dashboard UI stream.
*/
func (diagnostics *Diagnostics) publish() {
	if diagnostics.bus == nil {
		return
	}

	frame := diagnostics.Snapshot().Wire()

	diagnostics.feed.Emit(telemetry.Encode(&wire.FrameT{
		Type:  wire.FrameDiagnosticsFrame,
		Value: frame,
	}))

	diagnostics.feed.Emit(&types.UIFrame{
		Type:  wire.FrameDiagnosticsFrame,
		Value: frame,
	})
}
