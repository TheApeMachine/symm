package trader

import (
	"sync"
	"sync/atomic"
	"time"

	"github.com/spf13/viper"
	"github.com/theapemachine/datura"
	"github.com/theapemachine/symm/utils"
)

/*
durationClock accumulates one timed stage or hop. Average is total/count.
*/
type durationClock struct {
	count    atomic.Uint64
	totalNs  atomic.Uint64
	lastNs   atomic.Uint64
	maxNs    atomic.Uint64
	lastAtNs atomic.Int64
}

func (clock *durationClock) observe(duration time.Duration) {
	if clock == nil || duration < 0 {
		return
	}

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

func (clock *durationClock) snapshot(name string, kind string) ClockSnapshot {
	if clock == nil {
		return ClockSnapshot{Name: name, Kind: kind}
	}

	return ClockSnapshot{
		Name:     name,
		Kind:     kind,
		Count:    clock.count.Load(),
		TotalNs:  clock.totalNs.Load(),
		LastNs:   clock.lastNs.Load(),
		MaxNs:    clock.maxNs.Load(),
		LastAtNs: clock.lastAtNs.Load(),
	}
}

type clockBank struct {
	modules sync.Map
	hops    sync.Map
}

type diagnosticModule struct {
	name string
	kind string
}

var diagnosticModules = []diagnosticModule{
	{name: "price", kind: "broker"},
	{name: "desk", kind: "broker"},
	{name: "crypto", kind: "trader"},
	{name: "measurements", kind: "trader"},
	{name: "cvd", kind: "signal"},
	{name: "pumpdump", kind: "signal"},
	{name: "depthflow", kind: "signal"},
	{name: "exhaustion", kind: "signal"},
	{name: "hawkes", kind: "signal"},
	{name: "toxicity", kind: "signal"},
	{name: "correlation", kind: "signal"},
	{name: "leadlag", kind: "signal"},
	{name: "liquidity", kind: "signal"},
	{name: "sentiment", kind: "signal"},
	{name: "category-solver", kind: "logic"},
	{name: "resonance-solver", kind: "logic"},
	{name: "manifold-solver", kind: "logic"},
	{name: "causal-solver", kind: "logic"},
	{name: "cognition-solver", kind: "logic"},
	{name: "graph-solver", kind: "logic"},
	{name: "planner", kind: "strategy"},
	{name: "mcts", kind: "strategy"},
	{name: "allocation", kind: "strategy"},
}

var diagnosticHops = [][2]string{
	{"price", "crypto"},
	{"crypto", "measurements"},
	{"measurements", "category"},
	{"category", "causal"},
	{"causal", "graph"},
	{"graph", "planner"},
	{"planner", "mcts"},
	{"mcts", "allocation"},
	{"allocation", "desk"},
}

func (bank *clockBank) observe(name string, duration time.Duration) {
	if bank == nil {
		return
	}

	bank.module(name).observe(duration)
}

func (bank *clockBank) observeHop(from string, to string, duration time.Duration) {
	if bank == nil {
		return
	}

	bank.hop(from, to).observe(duration)
}

func (bank *clockBank) module(name string) *durationClock {
	if found, ok := bank.modules.Load(name); ok {
		return found.(*durationClock)
	}

	created := &durationClock{}
	actual, _ := bank.modules.LoadOrStore(name, created)

	return actual.(*durationClock)
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

/*
ClockSnapshot is one named accumulator on the diagnostics wire.
*/
type ClockSnapshot struct {
	Name     string `json:"name"`
	Kind     string `json:"kind"`
	Count    uint64 `json:"count"`
	TotalNs  uint64 `json:"total_ns"`
	LastNs   uint64 `json:"last_ns"`
	MaxNs    uint64 `json:"max_ns"`
	LastAtNs int64  `json:"last_at_ns"`
}

/*
HopSnapshot is the measured wait between two named stages.
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
LaneSnapshot remains on the wire so older dashboards keep parsing. The
measurement path no longer owns bounded SPSC rings.
*/
type LaneSnapshot struct {
	Name         string `json:"name"`
	Kind         string `json:"kind"`
	Blocking     bool   `json:"blocking"`
	Capacity     int    `json:"capacity"`
	Depth        int    `json:"depth"`
	HighWater    int64  `json:"high_water"`
	Saturations  uint64 `json:"saturations"`
	SaturationNs uint64 `json:"saturation_ns"`
}

/*
StreamDiagnostics is the replaceable wire snapshot for the diagnostics surface.
*/
type StreamDiagnostics struct {
	Status    string          `json:"status"`
	Summary   string          `json:"summary"`
	Lossy     bool            `json:"lossy"`
	AtNs      int64           `json:"at_ns"`
	StartedNs int64           `json:"started_ns"`
	Tickers   uint64          `json:"tickers"`
	Books     uint64          `json:"books"`
	Trades    uint64          `json:"trades"`
	Level3    uint64          `json:"level3"`
	UIDepth   int             `json:"ui_depth"`
	UICap     int             `json:"ui_cap"`
	UISent    uint64          `json:"ui_sent"`
	UIDropped uint64          `json:"ui_dropped"`
	Lanes     []LaneSnapshot  `json:"lanes"`
	Stages    []ClockSnapshot `json:"stages"`
	Hops      []HopSnapshot   `json:"hops"`
}

func (crypto *Crypto) bindDiagnostics() {
	if crypto == nil {
		return
	}

	crypto.startedAt = time.Now()
	crypto.diagnosticInterval = viper.GetDuration("system.bus.heartbeat")

	if crypto.diagnosticInterval <= 0 {
		crypto.diagnosticInterval = 250 * time.Millisecond
	}

	if crypto.measurements != nil {
		crypto.measurements.clocks = &crypto.clocks
	}

	if crypto.analyzer != nil {
		crypto.analyzer.ObserveModule = crypto.clocks.observe
		crypto.analyzer.ObserveHop = crypto.clocks.observeHop
	}

	if crypto.planner != nil {
		crypto.planner.ObserveModule = crypto.clocks.observe
		crypto.planner.ObserveHop = crypto.clocks.observeHop
	}

	if crypto.desk != nil {
		crypto.desk.ObserveModule = crypto.clocks.observe
	}

	go crypto.publishDiagnostics()
}

/*
Diagnostics snapshots stage clocks and UI backpressure for the measurement path.
*/
func (crypto *Crypto) Diagnostics() StreamDiagnostics {
	if crypto == nil {
		return StreamDiagnostics{Status: "flowing", Lanes: []LaneSnapshot{}}
	}

	sent, droppedUI := utils.PublishCounters()
	startedNs := int64(0)

	if !crypto.startedAt.IsZero() {
		startedNs = crypto.startedAt.UnixNano()
	}

	return StreamDiagnostics{
		Status:    "flowing",
		Summary:   "Measurement and analysis run inline on each market frame.",
		Lossy:     droppedUI > 0,
		AtNs:      time.Now().UnixNano(),
		StartedNs: startedNs,
		Tickers:   crypto.tickers.Load(),
		Trades:    crypto.trades.Load(),
		Level3:    crypto.level3.Load(),
		UIDepth:   uiDepth(crypto.ui),
		UICap:     uiCap(crypto.ui),
		UISent:    sent,
		UIDropped: droppedUI,
		Lanes:     []LaneSnapshot{},
		Stages:    crypto.stageSnapshots(),
		Hops:      crypto.hopSnapshots(),
	}
}

func (crypto *Crypto) stageSnapshots() []ClockSnapshot {
	stages := make([]ClockSnapshot, 0, len(diagnosticModules))

	for _, module := range diagnosticModules {
		stages = append(stages, crypto.clocks.module(module.name).snapshot(
			module.name,
			module.kind,
		))
	}

	return stages
}

func (crypto *Crypto) hopSnapshots() []HopSnapshot {
	hops := make([]HopSnapshot, 0, len(diagnosticHops))

	for _, pair := range diagnosticHops {
		snap := crypto.clocks.hop(pair[0], pair[1]).snapshot(pair[0]+"->"+pair[1], "")
		hops = append(hops, HopSnapshot{
			From:    pair[0],
			To:      pair[1],
			Count:   snap.Count,
			TotalNs: snap.TotalNs,
			LastNs:  snap.LastNs,
			MaxNs:   snap.MaxNs,
		})
	}

	return hops
}

func (crypto *Crypto) publishDiagnostics() {
	if crypto.ui == nil || crypto.diagnosticInterval <= 0 {
		return
	}

	ticker := time.NewTicker(crypto.diagnosticInterval)
	defer ticker.Stop()

	for {
		select {
		case <-crypto.ctx.Done():
			return
		case <-ticker.C:
			utils.Publish(crypto.ui, datura.NewMap(
				"diagnostics",
				crypto.Diagnostics(),
			))
		}
	}
}

func uiDepth(ui chan []byte) int {
	if ui == nil {
		return 0
	}

	return len(ui)
}

func uiCap(ui chan []byte) int {
	if ui == nil {
		return 0
	}

	return cap(ui)
}
