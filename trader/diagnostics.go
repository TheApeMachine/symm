package trader

import (
	"strconv"
	"sync"
	"sync/atomic"
	"time"

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
	{name: "collect", kind: "pipe"},
	{name: "commit", kind: "pipe"},
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
	{name: "category", kind: "logic"},
	{name: "resonance", kind: "logic"},
	{name: "manifold", kind: "logic"},
	{name: "causal", kind: "logic"},
	{name: "cognition", kind: "logic"},
	{name: "graph", kind: "logic"},
	{name: "planner", kind: "strategy"},
	{name: "mcts", kind: "strategy"},
	{name: "allocation", kind: "strategy"},
}

var diagnosticHops = [][2]string{
	{"price", "crypto"},
	{"crypto", "cvd"},
	{"crypto", "pumpdump"},
	{"crypto", "depthflow"},
	{"crypto", "exhaustion"},
	{"crypto", "hawkes"},
	{"crypto", "toxicity"},
	{"crypto", "correlation"},
	{"crypto", "leadlag"},
	{"crypto", "liquidity"},
	{"crypto", "sentiment"},
	{"signals", "collect"},
	{"collect", "commit"},
	{"commit", "category"},
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
LaneSnapshot is one bounded edge in the analytical data plane, named so a
blocked Dispatch or an idle commit can be told apart from a busy Measure.
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
It reports sequencer lag, lane pressure, and measured stage times.
*/
type StreamDiagnostics struct {
	Status            string          `json:"status"`
	Summary           string          `json:"summary"`
	Lossy             bool            `json:"lossy"`
	AtNs              int64           `json:"at_ns"`
	StartedNs         int64           `json:"started_ns"`
	IngressSequence   uint64          `json:"ingress_sequence"`
	CommittedSequence uint64          `json:"committed_sequence"`
	NextSequence      uint64          `json:"next_sequence"`
	Lag               uint64          `json:"lag"`
	Pending           int64           `json:"pending"`
	Dropped           uint64          `json:"dropped"`
	CommitDropped     uint64          `json:"commit_dropped"`
	Tickers           uint64          `json:"tickers"`
	Books             uint64          `json:"books"`
	Trades            uint64          `json:"trades"`
	Level3            uint64          `json:"level3"`
	CoalescedBooks    uint64          `json:"coalesced_books"`
	StallNs           uint64          `json:"stall_ns"`
	UIDepth           int             `json:"ui_depth"`
	UICap             int             `json:"ui_cap"`
	UISent            uint64          `json:"ui_sent"`
	UIDropped         uint64          `json:"ui_dropped"`
	Lanes             []LaneSnapshot  `json:"lanes"`
	Stages            []ClockSnapshot `json:"stages"`
	Hops              []HopSnapshot   `json:"hops"`
}

func (pipeline *streamPipeline) noteBroker(started time.Time) {
	if pipeline == nil {
		return
	}

	finished := time.Now()
	pipeline.clocks.observe("price", finished.Sub(started))
	pipeline.lastBrokerAt = finished
}

func (pipeline *streamPipeline) stampCollected(event *marketEvent) {
	handled := time.Now()

	if !event.measuredAt.IsZero() {
		pipeline.clocks.observeHop("signals", "collect", handled.Sub(event.measuredAt))
	}

	event.collectedAt = handled
}

/*
Diagnostics snapshots sequencer progress and every bounded lane without
waiting on collect or commit. The diagnostic publisher can therefore still
report a stall while those owners are parked.
*/
func (pipeline *streamPipeline) Diagnostics() StreamDiagnostics {
	ingress := pipeline.ingressSequence.Load()
	committed := pipeline.committedSequence.Load()
	lag := uint64(0)

	if ingress > committed {
		lag = ingress - committed
	}

	stallNs := uint64(0)

	if lag > 0 {
		lastCommit := pipeline.lastCommitNanos.Load()
		elapsed := time.Now().UnixNano() - lastCommit

		if lastCommit > 0 && elapsed > 0 {
			stallNs = uint64(elapsed)
		}
	}

	lanes := pipeline.laneSnapshots()
	blockedName := ""

	for _, lane := range lanes {
		if !lane.Blocking || lane.Capacity <= 0 || lane.Depth < lane.Capacity {
			continue
		}

		blockedName = lane.Name
		break
	}

	dropped := pipeline.dropped.Load()
	commitDropped := pipeline.commitDropped.Load()
	status, summary := diagnosticStatus(
		blockedName,
		lag,
		stallNs,
		pipeline.config.diagnosticInterval,
		pipeline.nextSequence.Load(),
		pipeline.pendingCount.Load(),
	)
	startedNs := int64(0)

	if !pipeline.startedAt.IsZero() {
		startedNs = pipeline.startedAt.UnixNano()
	}

	sent, droppedUI := utils.PublishCounters()

	return StreamDiagnostics{
		Status:            status,
		Summary:           summary,
		Lossy:             dropped > 0 || commitDropped > 0,
		AtNs:              time.Now().UnixNano(),
		StartedNs:         startedNs,
		IngressSequence:   ingress,
		CommittedSequence: committed,
		NextSequence:      pipeline.nextSequence.Load(),
		Lag:               lag,
		Pending:           pipeline.pendingCount.Load(),
		Dropped:           dropped,
		CommitDropped:     commitDropped,
		Tickers:           pipeline.tickers.Load(),
		Books:             pipeline.books.Load(),
		Trades:            pipeline.trades.Load(),
		Level3:            pipeline.level3.Load(),
		CoalescedBooks:    pipeline.coalescedBooks.Load(),
		StallNs:           stallNs,
		UIDepth:           uiDepth(pipeline.ui),
		UICap:             uiCap(pipeline.ui),
		UISent:            sent,
		UIDropped:         droppedUI,
		Lanes:             lanes,
		Stages:            pipeline.stageSnapshots(),
		Hops:              pipeline.hopSnapshots(),
	}
}

func (pipeline *streamPipeline) stageSnapshots() []ClockSnapshot {
	stages := make([]ClockSnapshot, 0, len(diagnosticModules))

	for _, module := range diagnosticModules {
		stages = append(stages, pipeline.clocks.module(module.name).snapshot(
			module.name,
			module.kind,
		))
	}

	return stages
}

func (pipeline *streamPipeline) hopSnapshots() []HopSnapshot {
	hops := make([]HopSnapshot, 0, len(diagnosticHops))

	for _, pair := range diagnosticHops {
		snap := pipeline.clocks.hop(pair[0], pair[1]).snapshot(pair[0]+"->"+pair[1], "")
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

func (pipeline *streamPipeline) publishDiagnostics(wait *sync.WaitGroup) {
	defer wait.Done()

	if pipeline.ui == nil || pipeline.config.diagnosticInterval <= 0 {
		return
	}

	ticker := time.NewTicker(pipeline.config.diagnosticInterval)
	defer ticker.Stop()

	for {
		select {
		case <-pipeline.ctx.Done():
			return
		case <-ticker.C:
			utils.Publish(pipeline.ui, datura.NewMap(
				"diagnostics",
				pipeline.Diagnostics(),
			))
		}
	}
}

func (pipeline *streamPipeline) laneSnapshots() []LaneSnapshot {
	lanes := make([]LaneSnapshot, 0, len(pipeline.workers)*2+1)

	for _, worker := range pipeline.workers {
		inboxKind := "local_inbox"

		if !worker.local {
			inboxKind = "cross_inbox"
		}

		lanes = append(lanes, snapshotLane(
			worker.name+".inbox",
			inboxKind,
			!worker.local,
			worker.inbox,
		))
		lanes = append(lanes, snapshotLane(
			worker.name+".outbox",
			"outbox",
			true,
			worker.outbox,
		))

		if worker.level3 != nil {
			lanes = append(lanes, snapshotLane(
				worker.name+".level3",
				"level3_inbox",
				false,
				worker.level3,
			))
		}
	}

	if pipeline.commitInbox != nil {
		lanes = append(lanes, snapshotLane(
			"commit",
			"commit",
			false,
			pipeline.commitInbox,
		))
	}

	return lanes
}

func snapshotLane[T any](
	name string,
	kind string,
	blocking bool,
	lane *lane[T],
) LaneSnapshot {
	if lane == nil {
		return LaneSnapshot{Name: name, Kind: kind, Blocking: blocking}
	}

	telemetry := lane.telemetry()

	return LaneSnapshot{
		Name:         name,
		Kind:         kind,
		Blocking:     blocking,
		Capacity:     telemetry.Capacity,
		Depth:        telemetry.Depth,
		HighWater:    telemetry.HighWater,
		Saturations:  telemetry.Saturations,
		SaturationNs: uint64(telemetry.SaturationDuration),
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

func diagnosticStatus(
	blockedName string,
	lag uint64,
	stallNs uint64,
	interval time.Duration,
	next uint64,
	pending int64,
) (string, string) {
	if blockedName != "" {
		return "stalled", "Lane " + blockedName +
			" is full; a blocking Push is parking its producer."
	}

	if lag > 0 && interval > 0 && stallNs >= uint64(interval) {
		return "stalled", "Commit is waiting; ticker sequence " +
			strconv.FormatUint(next, 10) + " is incomplete with " +
			strconv.FormatInt(pending, 10) + " events queued behind it."
	}

	return "flowing", "In-flight work is moving. A few events in a lane is the healthy plane, not a stall."
}
