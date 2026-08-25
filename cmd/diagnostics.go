package cmd

import (
	"bufio"
	"bytes"
	"errors"
	"runtime"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"strings"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/logic/graph"
	nomagiqueruntime "github.com/theapemachine/symm/nomagique/runtime"
	nmtypes "github.com/theapemachine/symm/nomagique/types"
	"github.com/theapemachine/symm/telemetry"
	wire "github.com/theapemachine/symm/telemetry/generated/telemetry"
	"github.com/theapemachine/symm/types"
)

/*
durationClock accumulates one timed stage. Average is total/count; lastNs is
the wall-clock of the most recent iteration; lastAtNs times the last activity
so the diagram can distinguish an actively running stage from an idle one.
*/
type durationClock struct {
	count    atomic.Uint64
	totalNs  atomic.Uint64
	lastNs   atomic.Uint64
	maxNs    atomic.Uint64
	lastAtNs atomic.Int64
	active   atomic.Uint64
	started  atomic.Int64
}

func (clock *durationClock) begin() {
	clock.active.Add(1)
	clock.started.Store(time.Now().UnixNano())
}

func (clock *durationClock) observe(duration time.Duration) {
	nanos := uint64(duration)
	clock.count.Add(1)
	clock.totalNs.Add(nanos)
	clock.lastNs.Store(nanos)
	clock.lastAtNs.Store(time.Now().UnixNano())

	for {
		current := clock.maxNs.Load()

		if nanos <= current || clock.maxNs.CompareAndSwap(current, nanos) {
			return
		}
	}
}

func (clock *durationClock) complete(duration time.Duration) {
	clock.observe(duration)

	for {
		active := clock.active.Load()

		if active == 0 || clock.active.CompareAndSwap(active, active-1) {
			return
		}
	}
}

func (clock *durationClock) snapshot(name string) ClockSnapshot {
	return ClockSnapshot{
		Name:      name,
		Count:     clock.count.Load(),
		TotalNs:   clock.totalNs.Load(),
		LastNs:    clock.lastNs.Load(),
		MaxNs:     clock.maxNs.Load(),
		LastAtNs:  clock.lastAtNs.Load(),
		Active:    clock.active.Load(),
		StartedNs: clock.started.Load(),
	}
}

/*
clockBank is the thread-safe store of stage clocks and the wired hops between
them. Every observe lands on the shared clock for the named stage; the diagram
reads one consistent snapshot per heartbeat.
*/
type clockBank struct {
	modules sync.Map
	hops    sync.Map
}

func (bank *clockBank) module(name string) *durationClock {
	if found, ok := bank.modules.Load(name); ok {
		return found.(*durationClock)
	}

	created := &durationClock{}
	actual, _ := bank.modules.LoadOrStore(name, created)

	return actual.(*durationClock)
}

func (bank *clockBank) observe(name string, duration time.Duration) {
	if name == "" {
		return
	}

	bank.module(name).observe(duration)
}

func (bank *clockBank) hop(from string, to string) *durationClock {
	key := from + "\x00" + to

	if found, ok := bank.hops.Load(key); ok {
		return found.(*durationClock)
	}

	created := &durationClock{}
	actual, _ := bank.hops.LoadOrStore(key, created)

	return actual.(*durationClock)
}

func (bank *clockBank) observeHop(from string, to string, duration time.Duration) {
	if from == "" || to == "" {
		return
	}

	bank.hop(from, to).observe(duration)
}

/*
ClockSnapshot is one named stage clock on the diagnostics wire.
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
HopSnapshot is the measured wait between two named stages — the wire latency
between systems, as opposed to the work time inside a stage.
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
ErrorSnapshot is one subsystem-attributed error for the idiot-proofing hint.
Source names the pipeline node the error belongs to; Caller keeps the phuslu
log caller so the diagram can point back at the offending code path.
*/
type ErrorSnapshot struct {
	Source  string `json:"source"`
	Message string `json:"message"`
	Caller  string `json:"caller"`
	AtNs    int64  `json:"at_ns"`
}

/*
PassStatus tells the diagram whether the measurement pass is gated idle (no
market rows pending), actively running, or blocked (a pass started but never
finished inside a signal/analyzer wait). This is the distinction that separates
"nothing to do" from "stuck doing nothing" — both otherwise look identical as a
climbing stage age on the upstream pipeline.
*/
type PassStatus struct {
	State       string `json:"state"`         // "idle" | "running" | "blocked"
	InFlightNs  int64  `json:"in_flight_ns"`  // current pass duration (0 when idle)
	LastPassNs  int64  `json:"last_pass_ns"`  // last completed pass duration
	SinceLastNs int64  `json:"since_last_ns"` // time since a pass last completed
}

/*
blockedPassThreshold is how long a single measurement pass may run before we
declare it blocked. A pass is one synchronous drain of every dirty symbol
through every signal plus one analyzer goroutine-group tick; on healthy feeds
this is well under a second, so anything persisting beyond this is a hang.
*/
const blockedPassThreshold = 2 * time.Second

/*
StreamDiagnostics is the wire snapshot of the whole wired analysis pipeline.
*/
type StreamDiagnostics struct {
	Status     string           `json:"status"`
	Enabled    bool             `json:"enabled"`
	AtNs       int64            `json:"at_ns"`
	StartedNs  int64            `json:"started_ns"`
	Stages     []ClockSnapshot  `json:"stages"`
	Hops       []HopSnapshot    `json:"hops"`
	Queues     []QueueSnapshot  `json:"queues"`
	Errors     []ErrorSnapshot  `json:"errors"`
	Pass       PassStatus       `json:"pass"`
	Goroutines []GoroutineOwner `json:"goroutines"`
}

/*
GoroutineOwner buckets the live goroutine inventory by the pipeline component
that owns each goroutine, so the dashboard can show which thing is running how
much instead of one opaque process-wide number.
*/
type GoroutineOwner struct {
	Owner string `json:"owner"`
	Count int64  `json:"count"`
	State string `json:"state"`
}

/*
QueueSnapshot is one observable queue's live pressure read on the diagnostics
wire. It names the stages that write into and read from the queue, its current
depth, and its capacity budget when bounded. Kind groups the queue by what it
carries so the diagram can color lanes semantically.
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

/*
Diagnostics owns the clock bank, consumes observations from every wired stage,
and publishes one snapshot per heartbeat onto the diagnostics WebRTC channel.
*/
type Diagnostics struct {
	clocks   clockBank
	started  time.Time
	interval time.Duration
	disabled atomic.Bool

	mutex sync.RWMutex
	errs  []ErrorSnapshot

	watermarks sync.Map // name → *atomic.Uint64 peak depth

	passMu        sync.Mutex
	passStart     time.Time
	passEnd       time.Time
	lastIdleCheck time.Time
	lastPassNs    atomic.Uint64
	blockedLogged bool

	routinesMu      sync.Mutex
	routinesAt      time.Time
	routineCounters []GoroutineOwner
}

/*
Enable resets the diagnostics switch to on. Observation hooks under normal
load only pay the single atomic load this method's checks add on the hot path.
*/
func (diagnostics *Diagnostics) Enable() {
	diagnostics.disabled.Store(false)
}

/*
Disable turns the diagnostics switch off. Every hot-path hook above returns
after one atomic load, and the heartbeat publisher drops to a slow idle cadence
so the pipeline incurs no per-observation or per-heartbeat collection cost.
*/
func (diagnostics *Diagnostics) Disable() {
	diagnostics.disabled.Store(true)
}

/*
Enabled reports whether diagnostics collection is currently switched on.
A nil receiver is not collecting, so it reports false. A zero-value receiver
defaults to enabled so unconfigured tests and short-lived call sites keep the
historical always-on behavior.
*/
func (diagnostics *Diagnostics) Enabled() bool {
	return diagnostics != nil && !diagnostics.disabled.Load()
}

/*
applyModule implements the ObserveModule callback signature used by analyzer,
measurements, planner, and desk.
*/
func (diagnostics *Diagnostics) applyModule(name string, duration time.Duration) {
	if !diagnostics.Enabled() {
		return
	}

	diagnostics.clocks.observe(name, duration)
}

func (diagnostics *Diagnostics) beginModule(name string) {
	if name == "" || !diagnostics.Enabled() {
		return
	}

	diagnostics.module(name).begin()
}

func (diagnostics *Diagnostics) completeModule(name string, duration time.Duration) {
	if name == "" || !diagnostics.Enabled() {
		return
	}

	diagnostics.module(name).complete(duration)
}

/*
applyHop implements the ObserveHop callback signature used by analyzer and
planner.
*/
func (diagnostics *Diagnostics) applyHop(from string, to string, duration time.Duration) {
	if !diagnostics.Enabled() {
		return
	}

	diagnostics.clocks.observeHop(from, to, duration)
}

/*
module returns the shared clock for a named stage.
*/
func (diagnostics *Diagnostics) module(name string) *durationClock {
	return diagnostics.clocks.module(name)
}

/*
ObserveError records a subsystem-attributed error for the idiot-proofing hint.
The most recent errors are kept so the diagram stays readable without flooding.
*/
func (diagnostics *Diagnostics) ObserveError(source string, message string, caller string) {
	if !diagnostics.Enabled() {
		return
	}

	diagnostics.mutex.Lock()
	defer diagnostics.mutex.Unlock()

	if source == "" {
		source = "system"
	}

	diagnostics.errs = append([]ErrorSnapshot{{
		Source:  source,
		Message: message,
		Caller:  caller,
		AtNs:    time.Now().UnixNano(),
	}}, diagnostics.errs...)

	if len(diagnostics.errs) > 16 {
		diagnostics.errs = diagnostics.errs[:16]
	}
}

/*
errorSnapshots returns a copy of the recorded errors.
*/
func (diagnostics *Diagnostics) errorSnapshots() []ErrorSnapshot {
	diagnostics.mutex.RLock()
	defer diagnostics.mutex.RUnlock()

	out := make([]ErrorSnapshot, len(diagnostics.errs))
	copy(out, diagnostics.errs)

	return out
}

/*
ObservePassStart marks the moment a measurement pass begins servicing pending
market rows. Until ObservePassEnd is called the pass is considered in-flight.
*/
func (diagnostics *Diagnostics) ObservePassStart(at time.Time) {
	if !diagnostics.Enabled() {
		return
	}

	diagnostics.passMu.Lock()
	defer diagnostics.passMu.Unlock()

	if diagnostics.passStart.IsZero() {
		diagnostics.passStart = at
	}
}

/*
ObservePassEnd marks the completion of a measurement pass and records how long
it took.
*/
func (diagnostics *Diagnostics) ObservePassEnd(at time.Time, duration time.Duration) {
	if !diagnostics.Enabled() {
		return
	}

	diagnostics.passMu.Lock()
	defer diagnostics.passMu.Unlock()

	diagnostics.passEnd = at
	diagnostics.passStart = time.Time{}
	diagnostics.lastPassNs.Store(uint64(duration))
	diagnostics.blockedLogged = false
}

/*
dumpBlockedStack logs a full goroutine dump the first time a pass crosses the
blocked threshold, so the offending signal or analyzer stage reveals itself by
its stack instead of the crash being silent. One dump per blocked episode.
*/
func (diagnostics *Diagnostics) dumpBlockedStack() {
	if diagnostics.blockedLogged {
		return
	}

	diagnostics.blockedLogged = true

	stack := make([]byte, 1<<20)
	n := runtime.Stack(stack, true)
	errnie.Error(errnie.Err(
		errnie.Timeout,
		"measurement pass blocked — dumping goroutines to identify the stuck stage",
		errors.New(string(stack[:n])),
	))
}

/*
ObserveIdleCheck records the last loop iteration that found no pending rows, so
the diagram can report the engine is gated idle rather than blocked.
*/
func (diagnostics *Diagnostics) ObserveIdleCheck(at time.Time) {
	if !diagnostics.Enabled() {
		return
	}

	diagnostics.passMu.Lock()
	defer diagnostics.passMu.Unlock()

	diagnostics.lastIdleCheck = at
}

/*
ObserveDiagnosticError forwards a subsystem-attributed error into the collector
so the diagnostics WebRTC frame can surface it on the wiring diagram. This is
the wire-safe sink the error bridge attaches to.
*/
func (crypto *Crypto) ObserveDiagnosticError(source string, message string, caller string) {
	if crypto == nil || crypto.diagnostics == nil {
		return
	}

	crypto.diagnostics.ObserveError(source, message, caller)
}

/*
Bind wires the diagnostics collector into analyzer, measurements, planner, and
desk observation hooks, then starts the heartbeat publisher over the diagnostics
WebRTC channel.
*/
func (crypto *Crypto) bindDiagnostics() {
	if crypto == nil || crypto.diagnostics == nil {
		return
	}

	if crypto.api != nil {
		crypto.api.SetObserver(crypto.diagnostics.applyModule)
	}

	if crypto.bus != nil {
		observeChannel(crypto.bus, types.ChannelTickers,
			func(ticker kraken.TickerData) string { return ticker.Symbol },
			crypto.diagnostics.beginModule, crypto.diagnostics.completeModule)
		observeChannel(crypto.bus, types.ChannelTrades,
			func(trade kraken.TradeData) string { return trade.Symbol },
			crypto.diagnostics.beginModule, crypto.diagnostics.completeModule)
		observeChannel(crypto.bus, types.ChannelLevel3,
			func(frame kraken.Level3Data) string { return frame.Symbol },
			crypto.diagnostics.beginModule, crypto.diagnostics.completeModule)
		observeChannel(crypto.bus, types.ChannelMeasurements,
			func(measurement *nmtypes.Measurement) string { return measurement.Symbol },
			crypto.diagnostics.beginModule, crypto.diagnostics.completeModule)
		observeChannel(crypto.bus, types.ChannelCategories,
			func(batch []types.Category) string {
				if len(batch) == 0 {
					return ""
				}
				return batch[0].Symbol
			},
			crypto.diagnostics.beginModule, crypto.diagnostics.completeModule)
		observeChannel(crypto.bus, types.ChannelResonance,
			func(artifact types.ResonanceArtifact) string { return artifact.Symbol },
			crypto.diagnostics.beginModule, crypto.diagnostics.completeModule)
		observeChannel(crypto.bus, types.ChannelCausal,
			func(output types.CausalOutput) string { return output.Symbol },
			crypto.diagnostics.beginModule, crypto.diagnostics.completeModule)
		observeChannel(crypto.bus, types.ChannelCognition,
			func(reading types.Cognition) string { return reading.Symbol },
			crypto.diagnostics.beginModule, crypto.diagnostics.completeModule)
		observeChannel(crypto.bus, types.ChannelRelations,
			func(update graph.GraphUpdate) string { return update.Symbol },
			crypto.diagnostics.beginModule, crypto.diagnostics.completeModule)

		// Forward every diagnostics heartbeat to the dashboard UI frame and the
		// manifold fluid stream. This is a side effect, so it rides the observer
		// hook: the Crypto owns its diagnostics end-to-end and does not depend on
		// the boot wiring to surface the frame to a viewer.
		crypto.bus.Observe(types.ChannelDiagnostics, func(_ string, value any) {
			diag, ok := value.(StreamDiagnostics)
			if !ok {
				return
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
		})
	}

	if crypto.diagnostics.interval <= 0 {
		crypto.diagnostics.interval = 250 * time.Millisecond
	}

	go crypto.publishDiagnostics()
}

/*
Diagnostics returns a snapshot of the whole wired pipeline for a heartbeat.
When the switch is off it returns only the status and toggle flag so the
frontend can show "disabled" without paying any collection cost.
*/
func (crypto *Crypto) Diagnostics() StreamDiagnostics {
	if crypto == nil || crypto.diagnostics == nil {
		return StreamDiagnostics{
			Status: "idle", Stages: []ClockSnapshot{}, Hops: []HopSnapshot{}, Errors: []ErrorSnapshot{},
		}
	}

	if !crypto.diagnostics.Enabled() {
		return StreamDiagnostics{
			Status:    "disabled",
			Enabled:   false,
			AtNs:      time.Now().UnixNano(),
			StartedNs: crypto.diagnostics.started.UnixNano(),
			Stages:    []ClockSnapshot{},
			Hops:      []HopSnapshot{},
			Queues:    []QueueSnapshot{},
			Errors:    []ErrorSnapshot{},
			Pass:      PassStatus{State: "idle"},
		}
	}

	return StreamDiagnostics{
		Status:     "flowing",
		Enabled:    true,
		AtNs:       time.Now().UnixNano(),
		StartedNs:  crypto.diagnostics.started.UnixNano(),
		Stages:     crypto.diagnostics.stageSnapshots(),
		Hops:       crypto.diagnostics.hopSnapshots(),
		Queues:     crypto.queueSnapshots(),
		Errors:     crypto.diagnostics.errorSnapshots(),
		Pass:       crypto.diagnostics.passStatus(time.Now()),
		Goroutines: crypto.diagnostics.goroutineInventory(),
	}
}

/*
queueSnapshots aggregates every observable stage buffer across the live symbol
universe into one pipeline-level pressure read. Per-symbol depths sum into each
logical queue so the diagram reports system-wide backpressure rather than a
per-ticker count. The writer/reader lists are static pipeline knowledge; the
depth values are read live on every heartbeat.
*/
func (crypto *Crypto) queueSnapshots() []QueueSnapshot {
	if crypto == nil || crypto.bus == nil {
		return []QueueSnapshot{}
	}

	tickers := channelSnapshot(crypto.bus, types.ChannelTickers,
		func(ticker kraken.TickerData) string { return ticker.Symbol })
	trades := channelSnapshot(crypto.bus, types.ChannelTrades,
		func(trade kraken.TradeData) string { return trade.Symbol })
	level3 := channelSnapshot(crypto.bus, types.ChannelLevel3,
		func(frame kraken.Level3Data) string { return frame.Symbol })
	measurements := channelSnapshot(crypto.bus, types.ChannelMeasurements,
		func(measurement *nmtypes.Measurement) string { return measurement.Symbol })
	categories := channelSnapshot(crypto.bus, types.ChannelCategories,
		func(batch []types.Category) string {
			if len(batch) == 0 {
				return ""
			}
			return batch[0].Symbol
		})
	resonance := channelSnapshot(crypto.bus, types.ChannelResonance,
		func(artifact types.ResonanceArtifact) string { return artifact.Symbol })
	causal := channelSnapshot(crypto.bus, types.ChannelCausal,
		func(output types.CausalOutput) string { return output.Symbol })
	cognition := channelSnapshot(crypto.bus, types.ChannelCognition,
		func(reading types.Cognition) string { return reading.Symbol })
	graphs := channelSnapshot(crypto.bus, types.ChannelRelations,
		func(update graph.GraphUpdate) string { return update.Symbol })
	ui := channelSnapshot(crypto.bus, types.ChannelUI,
		func(frame *types.UIFrame) string { return "" })
	fluid := channelSnapshot(crypto.bus, types.ChannelFluid,
		func(frame types.FluidFrame) string { return "" })

	collect := func(name string, kind string, writers []string, readers []string, snapshot nomagiqueruntime.WorkspaceSnapshot) QueueSnapshot {
		return QueueSnapshot{
			Name:      name,
			Kind:      kind,
			Writers:   writers,
			Readers:   readers,
			Depth:     snapshot.Pending,
			Cap:       snapshot.Capacity,
			HighWater: crypto.diagnostics.highWater(name, snapshot.Pending),
			Symbols:   snapshot.Lanes,
			Dropped:   snapshot.Dropped,
		}
	}

	return []QueueSnapshot{
		collect(
			"ingress.tickers", "ingress", []string{"ingress"},
			[]string{"correlation", "leadlag", "liquidity", "pumpdump", "sentiment", "resonance", "desk"},
			tickers,
		),
		collect(
			"ingress.trades", "ingress", []string{"ingress"},
			[]string{"cvd", "exhaustion", "hawkes", "pumpdump"},
			trades,
		),
		collect(
			"ingress.level3", "ingress", []string{"ingress"},
			[]string{"depthflow", "toxicity", "pumpdump"},
			level3,
		),
		collect(
			"measurements", "rail",
			[]string{"correlation", "cvd", "depthflow", "exhaustion", "hawkes", "leadlag", "liquidity", "pumpdump", "sentiment", "toxicity"},
			[]string{"category", "manifold", "graph"},
			measurements,
		),
		collect(
			"derived.category", "derived", []string{"category"},
			[]string{"graph", "cognition"},
			categories,
		),
		collect(
			"derived.causal", "derived", []string{"causal"},
			[]string{"graph"},
			causal,
		),
		collect(
			"derived.cognition", "derived", []string{"cognition"},
			[]string{"graph"},
			cognition,
		),
		collect(
			"derived.graph", "derived", []string{"graph"},
			[]string{"planner"},
			graphs,
		),
		collect(
			"derived.resonance", "derived", []string{"resonance"},
			[]string{"causal", "graph"},
			resonance,
		),
		{
			Name:      "ui.dashboard",
			Kind:      "ui",
			Writers:   []string{"ingress", "category", "manifold", "causal", "cognition", "graph", "resonance", "planner", "desk"},
			Readers:   []string{"hub"},
			Depth:     ui.Pending,
			Cap:       ui.Capacity,
			HighWater: crypto.diagnostics.highWater("ui.dashboard", ui.Pending),
			Dropped:   ui.Dropped,
		},
		{
			Name:      "ui.manifold",
			Kind:      "ui",
			Writers:   []string{"manifold", "diagnostics"},
			Readers:   []string{"webrtc-hub"},
			Depth:     fluid.Pending,
			Cap:       fluid.Capacity,
			HighWater: crypto.diagnostics.highWater("ui.manifold", fluid.Pending),
			Dropped:   fluid.Dropped,
		},
	}
}

/*
highWater tracks the running peak depth observed for a named queue. Each call
updates the peak if the new depth is higher and returns the stored peak.
*/
func (diagnostics *Diagnostics) highWater(name string, depth uint64) uint64 {
	if diagnostics == nil {
		return depth
	}

	entry, _ := diagnostics.watermarks.LoadOrStore(name, &atomic.Uint64{})
	watermark := entry.(*atomic.Uint64)

	for {
		current := watermark.Load()

		if depth <= current || watermark.CompareAndSwap(current, depth) {
			return watermark.Load()
		}
	}
}

/*
passStatus computes the measurement engine's state from the start/finish/idle
timestamps recorded by the Generate loop.
*/
func (diagnostics *Diagnostics) passStatus(now time.Time) PassStatus {
	diagnostics.passMu.Lock()
	defer diagnostics.passMu.Unlock()

	lastPass := diagnostics.lastPassNs.Load()

	if !diagnostics.passStart.IsZero() {
		inFlight := now.Sub(diagnostics.passStart)

		if inFlight > blockedPassThreshold {
			diagnostics.dumpBlockedStack()
			return PassStatus{
				State:      "blocked",
				InFlightNs: inFlight.Nanoseconds(),
				LastPassNs: int64(lastPass),
			}
		}

		return PassStatus{
			State:      "running",
			InFlightNs: inFlight.Nanoseconds(),
			LastPassNs: int64(lastPass),
		}
	}

	sinceLast := int64(0)

	if !diagnostics.passEnd.IsZero() {
		sinceLast = now.Sub(diagnostics.passEnd).Nanoseconds()
	}

	return PassStatus{
		State:       "idle",
		LastPassNs:  int64(lastPass),
		SinceLastNs: sinceLast,
	}
}

/*
publishDiagnostics sends one snapshot per heartbeat onto the diagnostics
WebRTC channel. When switched off it slows to an idle cadence and ships only
the tiny "disabled" status frame, so a viewer can still re-enable collection
while the pipeline pays nothing per heartbeat. No peer attached means the
manifold transport drops it — the frame is replaceable state, not a ledger.
*/
func (crypto *Crypto) publishDiagnostics() {
	if crypto == nil || crypto.diagnostics == nil {
		return
	}

	for {
		interval := crypto.diagnostics.interval

		if interval <= 0 {
			interval = 250 * time.Millisecond
		}

		if !crypto.diagnostics.Enabled() {
			interval = diagnosticsIdleInterval
		}

		timer := time.NewTimer(interval)

		select {
		case <-crypto.ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
			diag := crypto.Diagnostics()

			if crypto.bus != nil {
				crypto.bus.Publish(types.ChannelDiagnostics, diag)
			}
		}
	}
}

/*
diagnosticsIdleInterval is the heartbeat cadence while collection is switched
off: six times slower than the live cadence, still fast enough that a viewer
sees a toggle response within a beat or two.
*/
const diagnosticsIdleInterval = 1500 * time.Millisecond

/*
goroutineInventory buckets every live goroutine by the function that owns it,
refreshed at a slow cadence because a full runtime stack dump is the most
expensive diagnostics read by far. The owner is the first non-runtime frame,
so the dashboard reports "thesis", "kraken", "planner" and friends instead of
one process-wide total. It runs only while diagnostics are enabled; the caller
short-circuits it when the switch is off.
*/
const goroutineInventoryInterval = 2 * time.Second

func (diagnostics *Diagnostics) goroutineInventory() []GoroutineOwner {
	diagnostics.routinesMu.Lock()
	defer diagnostics.routinesMu.Unlock()

	now := time.Now()

	if now.Sub(diagnostics.routinesAt) < goroutineInventoryInterval {
		return diagnostics.routineCounters
	}

	stack := make([]byte, 1<<20)
	size := runtime.Stack(stack, true)

	if size >= len(stack) {
		stack = make([]byte, size*2)
		size = runtime.Stack(stack, true)
	}

	counts := make(map[string]GoroutineOwner)
	scanner := bufio.NewScanner(bytes.NewReader(stack[:size]))
	scanner.Buffer(make([]byte, 64*1024), 1<<20)

	var currentOwner string
	var currentState string

	for scanner.Scan() {
		line := scanner.Text()

		if strings.HasPrefix(line, "goroutine ") {
			if stateStart := strings.Index(line, "["); stateStart >= 0 {
				currentState = line[stateStart:]
			} else {
				currentState = ""
			}

			currentOwner = ""

			continue
		}

		if currentOwner != "" || currentState == "" {
			continue
		}

		function := strings.TrimSpace(line)

		if function == "" {
			continue
		}

		// Skip the runtime scheduler frames so the owner is the first
		// business frame that created or currently occupies this goroutine.
		if strings.HasPrefix(function, "runtime.") ||
			strings.HasPrefix(function, "internal/") {
			continue
		}

		if paren := strings.Index(function, "("); paren >= 0 {
			function = function[:paren]
		}

		currentOwner = ownerOf(function)
		entry := counts[currentOwner]
		entry.Owner = currentOwner
		entry.Count++
		entry.State = currentState
		counts[currentOwner] = entry
	}

	owners := make([]GoroutineOwner, 0, len(counts))

	for _, owner := range counts {
		owners = append(owners, owner)
	}

	sort.Slice(owners, func(first int, second int) bool {
		if owners[first].Count != owners[second].Count {
			return owners[first].Count > owners[second].Count
		}

		return owners[first].Owner < owners[second].Owner
	})

	diagnostics.routinesAt = now
	diagnostics.routineCounters = owners

	return owners
}

/*
ownerOf maps the first stack frame to the pipeline component it belongs to.
The mapping is derived from import paths rather than the pipeline's stage
names because goroutines live one level below the diagnostics stage labels.
*/
func ownerOf(function string) string {
	switch {
	case strings.Contains(function, "/kraken/"):
		return "kraken"
	case strings.Contains(function, "/signal/"):
		return "signals"
	case strings.Contains(function, "/logic/"):
		return "logic"
	case strings.Contains(function, "/strategy/"):
		return "strategy"
	case strings.Contains(function, "/broker/"):
		return "broker"
	case strings.Contains(function, "/ui/"):
		return "ui"
	case strings.Contains(function, "/trader/"):
		return "trader"
	case strings.Contains(function, "/types."):
		return "thesis"
	case strings.Contains(function, "/audit/"):
		return "audit"
	case strings.Contains(function, "/telemetry/"):
		return "telemetry"
	case strings.Contains(function, "/regulator/"):
		return "regulator"
	default:
		return function
	}
}

/*
stageSnapshots enumerates every observable pipeline node in wiring order. The
names match the strings emitted by the individual ObserveModule calls in
measurements.go, analyzer.go, planner.go, and desk.go.
*/
func (diagnostics *Diagnostics) stageSnapshots() []ClockSnapshot {
	out := make([]ClockSnapshot, 0)

	diagnostics.clocks.modules.Range(func(key, value any) bool {
		name := key.(string)
		clock := value.(*durationClock)
		out = append(out, clock.snapshot(name))
		return true
	})

	sort.Slice(out, func(i, j int) bool {
		return out[i].Name < out[j].Name
	})

	return out
}

/*
hopSnapshots enumerates the wired edges between stages in pipeline order.
*/
func (diagnostics *Diagnostics) hopSnapshots() []HopSnapshot {
	out := make([]HopSnapshot, 0)

	diagnostics.clocks.hops.Range(func(key, value any) bool {
		keyStr := key.(string)
		parts := strings.Split(keyStr, "\x00")
		if len(parts) != 2 {
			return true
		}

		clock := value.(*durationClock)
		out = append(out, HopSnapshot{
			From:    parts[0],
			To:      parts[1],
			Count:   clock.count.Load(),
			TotalNs: clock.totalNs.Load(),
			LastNs:  clock.lastNs.Load(),
			MaxNs:   clock.maxNs.Load(),
		})

		return true
	})

	sort.Slice(out, func(i, j int) bool {
		if out[i].From == out[j].From {
			return out[i].To < out[j].To
		}
		return out[i].From < out[j].From
	})

	return out
}

/*
observeChannel attaches the diagnostics clock to one typed bus channel.
*/
func observeChannel[T any](
	bus *nomagiqueruntime.Workspace,
	name string,
	_ func(T) string,
	begin func(string),
	end func(string, time.Duration),
) {
	if bus == nil {
		return
	}

	bus.Observe(name, func(_ string, _ any) {
		started := time.Now()
		if begin != nil {
			begin(name)
		}
		if end != nil {
			end(name, time.Since(started))
		}
	})
}

/*
channelSnapshot reads one typed bus channel's live pressure snapshot.
*/
func channelSnapshot[T any](
	bus *nomagiqueruntime.Workspace,
	name string,
	_ func(T) string,
) nomagiqueruntime.WorkspaceSnapshot {
	if bus == nil {
		return nomagiqueruntime.WorkspaceSnapshot{}
	}

	return bus.TopicSnapshot(name)
}
