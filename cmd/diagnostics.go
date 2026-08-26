package cmd

import (
	"sync"
	"sync/atomic"
	"time"

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
	count   atomic.Uint64
	totalNs atomic.Uint64
}

/*
ClockSnapshot is one node's live read: count and summed nanoseconds.
*/
type ClockSnapshot struct {
	Name      string `json:"name"`
	Count     uint64 `json:"count"`
	TotalNs   uint64 `json:"total_ns"`
	LastNs    uint64 `json:"last_ns"`
	MaxNs     uint64 `json:"max_ns"`
	LastAtNs  int64  `json:"last_at_ns"`
	Active    uint64 `json:"active"`
	StartedNs int64  `json:"started_ns"`
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
	Status     string          `json:"status"`
	Enabled    bool            `json:"enabled"`
	AtNs       int64           `json:"at_ns"`
	StartedNs  int64           `json:"started_ns"`
	Stages     []ClockSnapshot `json:"stages"`
	Queues     []QueueSnapshot `json:"queues"`
	Pass       PassStatus      `json:"pass"`
	Hops       []HopSnapshot   `json:"hops"`
	Errors     []ErrorSnapshot `json:"errors"`
	Goroutines []GoroutineOwner `json:"goroutines"`
}

/*
Diagnostics collects the two facts. Each node reports its Step time through
ObserveModule; each ring's occupancy is read directly from the workspace's
subscriber atomics on the heartbeat. There are no scanners, no stack walks, no
accumulators beyond the two sums, and no defensive copies.
*/
type Diagnostics struct {
	started time.Time
	// disabled is set when collection is switched off; the zero value keeps the
	// collector enabled so short-lived call sites and tests preserve the
	// historical always-on behavior without explicit configuration.
	disabled atomic.Bool

	// modules: node name -> {count, totalNs}. Written by ObserveModule on the
	// hot path with two atomic adds; read only on the heartbeat.
	modules sync.Map // name -> *stageClock

	interval time.Duration
}

func (diagnostics *Diagnostics) Enable()  { diagnostics.disabled.Store(false) }
func (diagnostics *Diagnostics) Disable() { diagnostics.disabled.Store(true) }
func (diagnostics *Diagnostics) Enabled() bool {
	return diagnostics != nil && !diagnostics.disabled.Load()
}

/*
applyModule sums one Step's duration into the node's running total. This is the
entire hot-path cost of diagnostics: two atomic adds, no allocation.
*/
func (diagnostics *Diagnostics) applyModule(name string, duration time.Duration) {
	if !diagnostics.Enabled() {
		return
	}

	clock := diagnostics.stage(name)
	clock.count.Add(1)
	clock.totalNs.Add(uint64(duration))
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
applyHop is a no-op compatibility hook for strategy stages that still report
edge timing; the stream model derives edge timing from the node clocks.
*/
func (diagnostics *Diagnostics) applyHop(from string, to string, duration time.Duration) {}

/*
stageSnapshots reads every node's two counters live. The result is a fresh slice
because the wire encode needs one; it is the single allocation the heartbeat
pays, not per-event work.
*/
func (diagnostics *Diagnostics) stageSnapshots() []ClockSnapshot {
	out := make([]ClockSnapshot, 0)

	diagnostics.modules.Range(func(key, value any) bool {
		clock := value.(*stageClock)
		out = append(out, ClockSnapshot{
			Name:    key.(string),
			Count:   clock.count.Load(),
			TotalNs: clock.totalNs.Load(),
		})

		return true
	})

	return out
}

/*
Diagnostics returns the live read for one heartbeat.
*/
func (crypto *Crypto) Diagnostics() StreamDiagnostics {
	diag := StreamDiagnostics{
		Status:     "flowing",
		Enabled:    true,
		AtNs:       time.Now().UnixNano(),
		StartedNs:  crypto.diagnostics.started.UnixNano(),
		Stages:     crypto.diagnostics.stageSnapshots(),
		Queues:     crypto.queueSnapshots(),
		Pass:       PassStatus{State: "idle"},
		Hops:       []HopSnapshot{},
		Errors:     []ErrorSnapshot{},
		Goroutines: []GoroutineOwner{},
	}

	if crypto == nil || crypto.diagnostics == nil || !crypto.diagnostics.Enabled() {
		diag.Status = "disabled"
		diag.Enabled = false
		diag.Stages = []ClockSnapshot{}
		diag.Queues = []QueueSnapshot{}
	}

	return diag
}

/*
queueSnapshots reads each ring's occupancy straight from the workspace: depth is
published-minus-completed (actual on-ring backlog), never a fabricated count.
Writer/reader wiring is the pipeline's static fan-out.
*/
func (crypto *Crypto) queueSnapshots() []QueueSnapshot {
	if crypto == nil || crypto.bus == nil {
		return []QueueSnapshot{}
	}

	collect := func(name string, kind string, writers []string, readers []string, topic string) QueueSnapshot {
		snapshot := crypto.bus.TopicSnapshot(topic)

		return QueueSnapshot{
			Name:    name,
			Kind:    kind,
			Writers: writers,
			Readers: readers,
			Depth:   snapshot.Pending,
			Cap:     runtime.SubscriberCapacity,
			Symbols: snapshot.Lanes,
			Dropped: snapshot.Dropped,
		}
	}

	return []QueueSnapshot{
		collect("ingress.tickers", "ingress", []string{"crypto"},
			[]string{"correlation", "leadlag", "liquidity", "pumpdump", "sentiment", "exhaustion", "resonance", "desk"},
			types.ChannelTickers),
		collect("ingress.trades", "ingress", []string{"crypto"},
			[]string{"cvd", "derivatives", "exhaustion", "hawkes", "pumpdump", "toxicity"},
			types.ChannelTrades),
		collect("ingress.level3", "ingress", []string{"crypto"},
			[]string{"depthflow"},
			types.ChannelLevel3),
		collect("measurements", "rail",
			[]string{"correlation", "cvd", "depthflow", "derivatives", "exhaustion", "hawkes", "leadlag", "liquidity", "pumpdump", "sentiment", "toxicity"},
			[]string{"category", "manifold", "graph"},
			types.ChannelMeasurements),
		collect("derived.category", "derived", []string{"category"},
			[]string{"cognition", "graph"}, types.ChannelCategories),
		collect("derived.causal", "derived", []string{"causal"},
			[]string{"graph"}, types.ChannelCausal),
		collect("derived.cognition", "derived", []string{"cognition"},
			[]string{"graph"}, types.ChannelCognition),
		collect("derived.graph", "derived", []string{"graph"},
			[]string{"planner"}, types.ChannelRelations),
		collect("derived.resonance", "derived", []string{"resonance"},
			[]string{"causal", "graph"}, types.ChannelResonance),
		collect("decisions", "strategy", []string{"planner"},
			[]string{"mcts", "allocation", "desk"}, types.ChannelDecisions),
		collect("desk.ticker", "broker", []string{"crypto"},
			[]string{"desk"}, types.ChannelTickers),
		collect("desk.executions", "broker", []string{"websocket-api"},
			[]string{"desk"}, types.ChannelExecutions),
		collect("positions", "broker", []string{"desk"},
			[]string{"audit", "hub"}, types.ChannelDecisions),
		collect("ui.dashboard", "ui",
			[]string{"crypto", "category", "manifold", "causal", "cognition", "graph", "resonance", "planner", "allocation", "desk", "diagnostics"},
			[]string{"hub"}, types.ChannelUI),
		collect("ui.manifold", "ui",
			[]string{"manifold", "diagnostics"},
			[]string{"webrtc-hub"}, types.ChannelFluid),
	}
}

/*
publishDiagnostics ships one live read per heartbeat.
*/
func (crypto *Crypto) publishDiagnostics() {
	if crypto == nil || crypto.diagnostics == nil {
		return
	}

	interval := crypto.diagnostics.interval
	if interval <= 0 {
		interval = 250 * time.Millisecond
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-crypto.ctx.Done():
			return
		case <-ticker.C:
			if !crypto.diagnostics.Enabled() {
				continue
			}

			if crypto.bus != nil {
				crypto.bus.Publish(types.ChannelDiagnostics, crypto.Diagnostics())
			}
		}
	}
}

/*
bindDiagnostics wires the diagnostics heartbeat into the dashboard. The live
StreamDiagnostics is projected to its FlatBuffers wire frame and forwarded to
both the fluid (WebRTC) stream and the dashboard UI stream, then the heartbeat
publisher starts. Ingress step time rides the api observer; ring depth and node
step time are read from the workspace at heartbeat time.
*/
func (crypto *Crypto) bindDiagnostics() {
	if crypto == nil || crypto.diagnostics == nil {
		return
	}

	if crypto.api != nil {
		crypto.api.SetObserver(crypto.diagnostics.applyModule)
	}

	if crypto.bus != nil {
		crypto.bus.Wire(types.ChannelDiagnostics, "", func(value any) any {
			diag, ok := value.(StreamDiagnostics)
			if !ok {
				return nil
			}

			diagWire := diag.Wire()

			crypto.bus.Publish(types.ChannelFluid, types.FluidFrame{
				Channel: types.DiagnosticsChannel,
				Payload: telemetry.Encode(&wire.FrameT{
					Type:  wire.FrameDiagnosticsFrame,
					Value: diagWire,
				}),
			})

			crypto.bus.Publish(types.ChannelUI, &types.UIFrame{
				Type:  wire.FrameDiagnosticsFrame,
				Value: diagWire,
			})

			return nil
		})
	}

	if crypto.diagnostics.interval <= 0 {
		crypto.diagnostics.interval = 250 * time.Millisecond
	}

	go crypto.publishDiagnostics()
}

/*
ObserveDiagnosticError is a no-op compatibility sink; the stream model surfaces
failures through the workspace failure handler rather than a diagnostics ledger.
*/
func (crypto *Crypto) ObserveDiagnosticError(source string, message string, caller string) {}
