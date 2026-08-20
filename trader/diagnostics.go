package trader

import (
	"errors"
	"runtime"
	"sync"
	"sync/atomic"
	"time"

	"github.com/spf13/viper"
	"github.com/theapemachine/datura"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/types"
	"github.com/theapemachine/symm/utils"
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

func (clock *durationClock) snapshot(name string) ClockSnapshot {
	return ClockSnapshot{
		Name:     name,
		Count:    clock.count.Load(),
		TotalNs:  clock.totalNs.Load(),
		LastNs:   clock.lastNs.Load(),
		MaxNs:    clock.maxNs.Load(),
		LastAtNs: clock.lastAtNs.Load(),
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
	Name     string `json:"name"`
	Count    uint64 `json:"count"`
	TotalNs  uint64 `json:"total_ns"`
	LastNs   uint64 `json:"last_ns"`
	MaxNs    uint64 `json:"max_ns"`
	LastAtNs int64  `json:"last_at_ns"`
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
	Status    string          `json:"status"`
	AtNs      int64           `json:"at_ns"`
	StartedNs int64           `json:"started_ns"`
	Stages    []ClockSnapshot `json:"stages"`
	Hops      []HopSnapshot   `json:"hops"`
	Queues    []QueueSnapshot `json:"queues"`
	Errors    []ErrorSnapshot `json:"errors"`
	Pass      PassStatus      `json:"pass"`
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
}

/*
Diagnostics owns the clock bank, consumes observations from every wired stage,
and publishes one snapshot per heartbeat onto the diagnostics WebRTC channel.
*/
type Diagnostics struct {
	clocks   clockBank
	started  time.Time
	interval time.Duration

	mutex sync.RWMutex
	errs  []ErrorSnapshot

	watermarks sync.Map // name → *atomic.Uint64 peak depth

	passMu        sync.Mutex
	passStart     time.Time
	passEnd       time.Time
	lastIdleCheck time.Time
	lastPassNs    atomic.Uint64
	blockedLogged bool
}

/*
applyModule implements the ObserveModule callback signature used by analyzer,
measurements, planner, and desk.
*/
func (diagnostics *Diagnostics) applyModule(name string, duration time.Duration) {
	diagnostics.clocks.observe(name, duration)
}

/*
applyHop implements the ObserveHop callback signature used by analyzer and
planner.
*/
func (diagnostics *Diagnostics) applyHop(from string, to string, duration time.Duration) {
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

	if crypto.analyzer != nil {
		crypto.analyzer.ObserveModule = crypto.diagnostics.applyModule
		crypto.analyzer.ObserveHop = crypto.diagnostics.applyHop
	}

	if crypto.planner != nil {
		crypto.planner.ObserveModule = crypto.diagnostics.applyModule
		crypto.planner.ObserveHop = crypto.diagnostics.applyHop
	}

	if crypto.desk != nil {
		crypto.desk.ObserveModule = crypto.diagnostics.applyModule
	}

	if crypto.diagnostics.interval <= 0 {
		crypto.diagnostics.interval = 250 * time.Millisecond
	}

	go crypto.publishDiagnostics()
}

/*
Diagnostics returns a snapshot of the whole wired pipeline for a heartbeat.
*/
func (crypto *Crypto) Diagnostics() StreamDiagnostics {
	if crypto == nil || crypto.diagnostics == nil {
		return StreamDiagnostics{
			Status: "idle", Stages: []ClockSnapshot{}, Hops: []HopSnapshot{}, Errors: []ErrorSnapshot{},
		}
	}

	return StreamDiagnostics{
		Status:    "flowing",
		AtNs:      time.Now().UnixNano(),
		StartedNs: crypto.diagnostics.started.UnixNano(),
		Stages:    crypto.diagnostics.stageSnapshots(),
		Hops:      crypto.diagnostics.hopSnapshots(),
		Queues:    crypto.queueSnapshots(),
		Errors:    crypto.diagnostics.errorSnapshots(),
		Pass:      crypto.diagnostics.passStatus(time.Now()),
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
	if crypto == nil || crypto.thesis == nil {
		return []QueueSnapshot{}
	}

	// Aggregate every per-symbol stage buffer depth once per heartbeat.
	perSymbol := make(map[string]uint64)
	symbolCount := make(map[string]uint64)

	crypto.thesis.Symbols.Range(func(_ any, value any) bool {
		symbol, ok := value.(*types.Symbol)

		if !ok || symbol == nil {
			return true
		}

		for key, depth := range symbol.QueueDepths() {
			symbolCount[key]++
			perSymbol[key] += depth
		}

		return true
	})

	collect := func(name string, kind string, writers []string, readers []string, key string) QueueSnapshot {
		depth := perSymbol[key]
		return QueueSnapshot{
			Name:      name,
			Kind:      kind,
			Writers:   writers,
			Readers:   readers,
			Depth:     depth,
			Cap:       0, // per-symbol stage buffers are unbounded
			HighWater: crypto.diagnostics.highWater(name, depth),
			Symbols:   symbolCount[key],
		}
	}

	// Shared transport depths are not per-symbol, so they are read directly.
	var uiDashDepth uint64
	var uiManifoldDepth uint64

	if crypto.ui != nil {
		uiDashDepth = crypto.ui.Length()
	}

	if crypto.manifold != nil {
		uiManifoldDepth = crypto.manifold.Length()
	}

	// Broker subscription channels are bounded, so their cap is the buffer size.
	brokerCap := uint64(1024)

	if viper.GetInt("system.actor.buffer") > 0 {
		brokerCap = uint64(viper.GetInt("system.actor.buffer"))
	}

	var tickerDepth uint64
	var executionsDepth uint64

	if crypto.desk != nil {
		tickerDepth = uint64(crypto.desk.QueueDepth("ticker"))
		executionsDepth = uint64(crypto.desk.QueueDepth("executions"))
	}

	return []QueueSnapshot{
		collect(
			"ingress.tickers", "ingress", []string{"ingress"},
			[]string{"correlation", "cvd", "leadlag", "liquidity", "pumpdump", "sentiment"},
			"tickers",
		),
		collect(
			"ingress.trades", "ingress", []string{"ingress"},
			[]string{"cvd", "depthflow", "exhaustion", "hawkes", "pumpdump", "toxicity"},
			"trades",
		),
		collect(
			"ingress.level3", "ingress", []string{"ingress"},
			[]string{"toxicity", "pumpdump"},
			"level3",
		),
		collect(
			"measurements", "rail",
			[]string{"correlation", "cvd", "depthflow", "exhaustion", "hawkes", "leadlag", "liquidity", "pumpdump", "sentiment", "toxicity"},
			[]string{"category", "manifold", "graph"},
			"measurements",
		),
		collect(
			"derived.category", "derived", []string{"category"},
			[]string{"graph", "cognition"},
			"categories",
		),
		collect(
			"derived.causal", "derived", []string{"causal"},
			[]string{"graph", "causal"},
			"causal",
		),
		collect(
			"derived.cognition", "derived", []string{"cognition"},
			[]string{"graph"},
			"cognition",
		),
		collect(
			"derived.graph", "derived", []string{"graph"},
			[]string{"planner", "graph"},
			"graphs",
		),
		collect(
			"derived.resonance", "derived", []string{"resonance"},
			[]string{"causal", "graph"},
			"resonance",
		),
		collect(
			"decisions", "strategy", []string{"planner"},
			[]string{"audit"},
			"decisions",
		),
		collect(
			"positions", "strategy", []string{"desk"},
			[]string{"audit"},
			"positions",
		),
		{
			Name:      "ui.dashboard",
			Kind:      "ui",
			Writers:   []string{"category", "manifold", "causal", "cognition", "graph", "resonance", "planner", "desk"},
			Readers:   []string{"hub"},
			Depth:     uiDashDepth,
			Cap:       0,
			HighWater: crypto.diagnostics.highWater("ui.dashboard", uiDashDepth),
		},
		{
			Name:      "ui.manifold",
			Kind:      "ui",
			Writers:   []string{"manifold", "resonance", "diagnostics"},
			Readers:   []string{"webrtc-hub"},
			Depth:     uiManifoldDepth,
			Cap:       0,
			HighWater: crypto.diagnostics.highWater("ui.manifold", uiManifoldDepth),
		},
		{
			Name:      "desk.ticker",
			Kind:      "broker",
			Writers:   []string{"websocket-api"},
			Readers:   []string{"desk"},
			Depth:     tickerDepth,
			Cap:       brokerCap,
			HighWater: crypto.diagnostics.highWater("desk.ticker", tickerDepth),
		},
		{
			Name:      "desk.executions",
			Kind:      "broker",
			Writers:   []string{"websocket-api"},
			Readers:   []string{"desk"},
			Depth:     executionsDepth,
			Cap:       brokerCap,
			HighWater: crypto.diagnostics.highWater("desk.executions", executionsDepth),
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
publishDiagnostics sends one snapshot every heartbeat onto the diagnostics
WebRTC channel. No peer attached means the manifold transport drops it — the
frame is replaceable state, not a ledger, and an empty canvas costs nothing.
*/
func (crypto *Crypto) publishDiagnostics() {
	if crypto == nil || crypto.diagnostics == nil {
		return
	}

	ticker := time.NewTicker(crypto.diagnostics.interval)
	defer ticker.Stop()

	for {
		select {
		case <-crypto.ctx.Done():
			return
		case <-ticker.C:
			utils.PublishFluid(
				crypto.manifold,
				types.DiagnosticsChannel,
				datura.NewMap("diagnostics", crypto.Diagnostics()),
			)
		}
	}
}

/*
stageNames is the fixed ordering of observable pipeline nodes.
*/
func stageNames() []string {
	return []string{
		"crypto",
		"correlation", "cvd", "depthflow", "exhaustion",
		"hawkes", "leadlag", "liquidity", "pumpdump",
		"sentiment", "toxicity",
		"category", "manifold", "causal", "cognition", "graph",
		"resonance",
		"planner", "mcts", "allocation",
		"desk",
	}
}

/*
stageSnapshots enumerates every observable pipeline node in wiring order. The
names match the strings emitted by the individual ObserveModule calls in
measurements.go, analyzer.go, planner.go, and desk.go.
*/
func (diagnostics *Diagnostics) stageSnapshots() []ClockSnapshot {
	names := stageNames()

	out := make([]ClockSnapshot, 0, len(names))

	for _, name := range names {
		out = append(out, diagnostics.module(name).snapshot(name))
	}

	return out
}

/*
hopSnapshots enumerates the wired edges between stages in pipeline order.
*/
func (diagnostics *Diagnostics) hopSnapshots() []HopSnapshot {
	edges := [][2]string{
		{"crypto", "measurements"},
		{"measurements", "category"},
		{"category", "causal"},
		{"causal", "graph"},
		{"graph", "planner"},
		{"planner", "mcts"},
		{"mcts", "allocation"},
		{"allocation", "desk"},
	}

	out := make([]HopSnapshot, 0, len(edges))

	for _, edge := range edges {
		clock := diagnostics.clocks.hop(edge[0], edge[1])
		out = append(out, HopSnapshot{
			From:    edge[0],
			To:      edge[1],
			Count:   clock.count.Load(),
			TotalNs: clock.totalNs.Load(),
			LastNs:  clock.lastNs.Load(),
			MaxNs:   clock.maxNs.Load(),
		})
	}

	return out
}
