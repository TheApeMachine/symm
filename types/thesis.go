package types

import (
	"context"
	"encoding/json"
	"fmt"
	"runtime"
	"sync"
	"sync/atomic"
	"time"

	pmanifold "github.com/theapemachine/nomagique/physics/manifold"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/nomagique/transport"
	wire "github.com/theapemachine/symm/telemetry/generated/telemetry"
)

/*
Thesis owns canonical evidence across every evaluated epoch that contributes to
one decision. It closes only after the planner emits the completed decision set;
broker execution and settlement continue in their own lifecycle.
*/
type Thesis struct {
	ctx            context.Context
	cancel         context.CancelFunc
	ui             *transport.MapReduce[*UIFrame]
	balance        kraken.BalanceData
	equityMu       sync.RWMutex
	equity         *kraken.TradeBalanceResult
	equityRevision uint64
	clockMu        sync.Mutex
	symbolMu       sync.Mutex
	symbols        []*Symbol
	nextSymbolID   SymbolID
	Status         Status          `json:"status"`
	Tick           int64           `json:"tick"`
	At             time.Time       `json:"at"`
	CrossSection   *CrossSection   `json:"crossSection"`
	Symbols        *sync.Map       `json:"-"`
	Audit          func(any) error `json:"-"`
	manifold       atomic.Pointer[pmanifold.Reading]
	phase          atomic.Pointer[PhaseReading]
	work           map[SourceType]*transport.MapReduce[*Symbol]
	workRevision   *atomic.Uint64
	workHeld       *atomic.Uint64
	workDeferred   *atomic.Uint64
	failure        func(error)
}

func (thesis *Thesis) SetFailureHandler(handler func(error)) {
	thesis.failure = handler
}

func (thesis *Thesis) Fail(err error) {
	if err != nil && thesis.failure != nil {
		thesis.failure(err)
	}
}

/*
AdvanceTick commits one real market observation to the engine clock and
publishes that exact clock transition to the focused dashboard stream.
*/
func (thesis *Thesis) AdvanceTick(at time.Time) int64 {
	if thesis == nil {
		panic("thesis: market clock required")
	}

	thesis.clockMu.Lock()
	thesis.Tick++
	thesis.At = at
	tick := thesis.Tick
	thesis.clockMu.Unlock()
	thesis.ScheduleWork(SourcePlanner, nil)

	thesis.Publish(&UIFrame{
		Type: wire.FrameTickFrame,
		Value: &wire.TickFrameT{
			Count: tick,
		},
	})

	return tick
}

/*
NewThesis creates a Thesis with empty durable maps and no tick evidence yet.
*/
func NewThesis(
	ctx context.Context, ui *transport.MapReduce[*UIFrame],
) *Thesis {
	ctx, cancel := context.WithCancel(ctx)
	work := make(map[SourceType]*transport.MapReduce[*Symbol], len(WorkerSources))
	workRevision := &atomic.Uint64{}
	workHeld := &atomic.Uint64{}
	workDeferred := &atomic.Uint64{}

	for _, source := range WorkerSources {
		work[source] = transport.NewMapReduce[*Symbol](nil, nil, nil)
	}

	return &Thesis{
		ctx:          ctx,
		cancel:       cancel,
		ui:           ui,
		Status:       READY,
		CrossSection: NewCrossSection(),
		Symbols:      &sync.Map{},
		work:         work,
		workRevision: workRevision,
		workHeld:     workHeld,
		workDeferred: workDeferred,
	}
}

/*
Work returns the lossless ready-symbol queue owned by one running stage.
MapReduce producers append only on an input reader's empty-to-ready transition.
*/
func (thesis *Thesis) Work(source SourceType) *transport.MapReduce[*Symbol] {
	if thesis == nil {
		panic("thesis: work scheduler required")
	}

	work, found := thesis.work[source]

	if !found || work == nil {
		panic("thesis: unsupported work source " + string(source))
	}

	return work
}

/*
ScheduleWork publishes one worker transition and advances the shared progress
revision after the queue has accepted it.
*/
func (thesis *Thesis) ScheduleWork(source SourceType, symbol *Symbol) {
	if thesis.workIsHeld(source) {
		thesis.workDeferred.Or(deferredWorkBit(source))

		return
	}

	thesis.Work(source).Push(symbol)
	thesis.workRevision.Add(1)
}

/*
HoldWork defers selected derived stages at an external observation boundary.
The source input queues remain authoritative; releasing the stage schedules
exactly the symbols whose existing consumer cursor still has pending input.
*/
func (thesis *Thesis) HoldWork(sources ...SourceType) {
	if thesis == nil || thesis.workHeld == nil {
		panic("thesis: work scheduler required")
	}

	for _, source := range sources {
		bit := deferredWorkBit(source)
		thesis.workHeld.Or(bit)
	}
}

/*
ReleaseWork re-enables one held stage and schedules its retained symbol input.
Ingress must be fenced while a held stage is released so the retained cut has
one unambiguous external boundary.
*/
func (thesis *Thesis) ReleaseWork(source SourceType) {
	if thesis == nil || thesis.workHeld == nil {
		panic("thesis: work scheduler required")
	}

	bit := deferredWorkBit(source)
	previous := thesis.workHeld.And(^bit)

	if previous&bit == 0 {
		return
	}

	deferred := thesis.workDeferred.And(^bit)&bit != 0

	if !deferred {
		return
	}

	pending := false
	crossSection := source == SourceManifold || source == SourcePlanner
	thesis.symbolMu.Lock()

	for _, symbol := range thesis.symbols {
		if symbol == nil || !symbol.HasPendingWork(source) {
			continue
		}

		pending = true

		if !crossSection {
			thesis.ScheduleWork(source, symbol)
		}
	}

	thesis.symbolMu.Unlock()

	if source == SourceManifold && pending {
		thesis.ScheduleWork(SourceManifold, nil)
	}

	if source != SourcePlanner {
		return
	}

	if pending || deferred {
		thesis.ScheduleWork(SourcePlanner, nil)
	}
}

func (thesis *Thesis) workIsHeld(source SourceType) bool {
	if thesis == nil || thesis.workHeld == nil {
		return false
	}

	bit, deferred := deferredWorkBitOK(source)

	return deferred && thesis.workHeld.Load()&bit != 0
}

func deferredWorkBit(source SourceType) uint64 {
	bit, valid := deferredWorkBitOK(source)

	if !valid {
		panic("thesis: source cannot be deferred: " + string(source))
	}

	return bit
}

func deferredWorkBitOK(source SourceType) (uint64, bool) {
	switch source {
	case SourceCorrelation:
		return 1 << 0, true
	case SourceCVD:
		return 1 << 1, true
	case SourceDepthFlow:
		return 1 << 2, true
	case SourceExhaustion:
		return 1 << 3, true
	case SourceHawkes:
		return 1 << 4, true
	case SourceLeadLag:
		return 1 << 5, true
	case SourceLiquidity:
		return 1 << 6, true
	case SourcePumpDump:
		return 1 << 7, true
	case SourceSentiment:
		return 1 << 8, true
	case SourceToxicity:
		return 1 << 9, true
	case SourceResonance:
		return 1 << 10, true
	case SourceCategory:
		return 1 << 11, true
	case SourceManifold:
		return 1 << 12, true
	case SourceCausal:
		return 1 << 13, true
	case SourceCognition:
		return 1 << 14, true
	case SourceGraph:
		return 1 << 15, true
	case SourcePlanner:
		return 1 << 16, true
	default:
		return 0, false
	}
}

/*
WaitForQuiescence waits for a stable fixed point across every worker queue.
Ingress must be fenced before calling it so no external producer can begin a
new generation after the fixed point is observed.
*/
func (thesis *Thesis) WaitForQuiescence(ctx context.Context) error {
	for {
		revision := thesis.workRevision.Load()
		idle := true

		for _, source := range WorkerSources {
			if !thesis.Work(source).Idle() {
				idle = false
				break
			}
		}

		if idle && thesis.workRevision.Load() == revision {
			return nil
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			runtime.Gosched()
		}
	}
}

func (thesis *Thesis) notifyWork(source SourceType, symbol *Symbol) {
	if thesis.workIsHeld(source) {
		thesis.workDeferred.Or(deferredWorkBit(source))

		return
	}

	work, found := thesis.work[source]

	if !found {
		return
	}

	work.Push(symbol)
	thesis.workRevision.Add(1)
}

func (thesis *Thesis) UI() *transport.MapReduce[*UIFrame] {
	return thesis.ui
}

/*
Publish appends one marshaled dashboard wire frame to the lock-free UI
transport. The frame is retained until the hub consumer drains it; a producer
never blocks and never drops, regardless of transport backpressure.
*/
func (thesis *Thesis) Publish(frame *UIFrame) {
	if thesis == nil || thesis.ui == nil {
		return
	}

	thesis.ui.Push(frame)
}

func (thesis *Thesis) Symbol(name string) *Symbol {
	symbol, ok := thesis.Symbols.Load(name)

	if ok {
		return symbol.(*Symbol)
	}

	thesis.symbolMu.Lock()
	defer thesis.symbolMu.Unlock()

	symbol, ok = thesis.Symbols.Load(name)

	if !ok {
		created := NewSymbol(name, thesis.ui)
		created.setNotify(thesis.notifyWork)
		created.ID = thesis.nextSymbolID
		thesis.nextSymbolID++
		thesis.symbols = append(thesis.symbols, created)
		symbol = created
		thesis.Symbols.Store(name, symbol)
	}

	return symbol.(*Symbol)
}

/*
ForSymbol returns a non-owning analytical view containing exactly one existing
symbol. The view shares the symbol and cross-sectional evidence with its parent
while preventing incremental solvers from rescanning unrelated symbols.
*/
func (thesis *Thesis) ForSymbol(name string) (*Thesis, error) {
	if thesis == nil || name == "" {
		return nil, fmt.Errorf("thesis: symbol scope required")
	}

	symbol, found := thesis.Symbols.Load(name)

	if !found {
		return nil, fmt.Errorf("thesis: symbol scope not found: %s", name)
	}

	symbols := &sync.Map{}
	symbols.Store(name, symbol)

	scoped := &Thesis{
		ctx:          thesis.ctx,
		ui:           thesis.ui,
		Status:       thesis.Status,
		Tick:         thesis.Tick,
		At:           thesis.At,
		CrossSection: thesis.CrossSection,
		Symbols:      symbols,
		symbols:      []*Symbol{symbol.(*Symbol)},
		Audit:        thesis.Audit,
		work:         thesis.work,
		workRevision: thesis.workRevision,
		workHeld:     thesis.workHeld,
		workDeferred: thesis.workDeferred,
	}
	manifold := thesis.manifold.Load()

	if manifold != nil {
		scoped.manifold.Store(manifold)
	}

	phase := thesis.phase.Load()

	if phase != nil {
		scoped.phase.Store(phase)
	}

	return scoped, nil
}

/*
ForSymbols returns a non-owning analytical view containing exactly the named
existing symbols. The view shares those symbols and the cross-section with its
parent so one admission round can rank a batch without scanning the universe.
*/
func (thesis *Thesis) ForSymbols(names []string) (*Thesis, error) {
	if thesis == nil || len(names) == 0 {
		return nil, fmt.Errorf("thesis: symbol scope required")
	}

	symbols := &sync.Map{}
	ordered := make([]*Symbol, 0, len(names))

	for _, name := range names {
		if name == "" {
			return nil, fmt.Errorf("thesis: symbol scope required")
		}

		symbol, found := thesis.Symbols.Load(name)

		if !found {
			return nil, fmt.Errorf("thesis: symbol scope not found: %s", name)
		}

		symbols.Store(name, symbol)
		ordered = append(ordered, symbol.(*Symbol))
	}

	scoped := &Thesis{
		ctx:          thesis.ctx,
		ui:           thesis.ui,
		Status:       thesis.Status,
		Tick:         thesis.Tick,
		At:           thesis.At,
		CrossSection: thesis.CrossSection,
		Symbols:      symbols,
		symbols:      ordered,
		Audit:        thesis.Audit,
		work:         thesis.work,
		workRevision: thesis.workRevision,
		workHeld:     thesis.workHeld,
		workDeferred: thesis.workDeferred,
	}
	manifold := thesis.manifold.Load()

	if manifold != nil {
		scoped.manifold.Store(manifold)
	}

	phase := thesis.phase.Load()

	if phase != nil {
		scoped.phase.Store(phase)
	}

	return scoped, nil
}

/*
StoreManifold atomically publishes one immutable fluid reading.
*/
func (thesis *Thesis) StoreManifold(reading pmanifold.Reading) {
	thesis.manifold.Store(&reading)
}

/*
ManifoldSnapshot returns the latest complete fluid reading when one exists.
*/
func (thesis *Thesis) ManifoldSnapshot() (pmanifold.Reading, bool) {
	reading := thesis.manifold.Load()

	if reading == nil {
		return pmanifold.Reading{}, false
	}

	return *reading, true
}

/*
StorePhase atomically publishes one immutable universe phase sweep.
*/
func (thesis *Thesis) StorePhase(reading PhaseReading) {
	thesis.phase.Store(&reading)
}

/*
PhaseSnapshot returns the latest complete universe phase sweep when one exists.
*/
func (thesis *Thesis) PhaseSnapshot() (PhaseReading, bool) {
	reading := thesis.phase.Load()

	if reading == nil {
		return PhaseReading{}, false
	}

	return *reading, true
}

/*
SymbolID is the append-only numerical identity assigned on first observation.
*/
type SymbolID uint32

func (thesis *Thesis) AppendEquity(equity kraken.TradeBalanceResult) error {
	if equity.Equity == nil || equity.Equity.Sign() <= 0 {
		return fmt.Errorf("thesis: positive equity required")
	}

	thesis.equityMu.Lock()
	defer thesis.equityMu.Unlock()
	thesis.equity = &equity
	thesis.equityRevision++

	return nil
}

/*
EquitySnapshot returns the latest complete valuation and its monotonic revision.
The revision lets the regulator spend each broker observation at most once.
*/
func (thesis *Thesis) EquitySnapshot() (
	kraken.TradeBalanceResult,
	uint64,
	bool,
) {
	thesis.equityMu.RLock()
	defer thesis.equityMu.RUnlock()

	if thesis.equity == nil {
		return kraken.TradeBalanceResult{}, 0, false
	}

	return *thesis.equity, thesis.equityRevision, true
}

func (thesis *Thesis) Equity() (kraken.TradeBalanceResult, bool) {
	thesis.equityMu.RLock()
	defer thesis.equityMu.RUnlock()

	if thesis.equity == nil {
		return kraken.TradeBalanceResult{}, false
	}

	return *thesis.equity, true
}

func (thesis *Thesis) MarshalState() ([]byte, error) {
	symbols := make(map[string]any)

	thesis.Symbols.Range(func(key, value any) bool {
		name, valid := key.(string)

		if !valid || name == "" {
			return true
		}

		symbol, valid := value.(*Symbol)

		if !valid || symbol == nil {
			return true
		}

		symbols[name] = symbol.CheckpointState()
		return true
	})
	equity, _, hasEquity := thesis.EquitySnapshot()
	checkpoint := struct {
		Status       Status                     `json:"status"`
		Tick         int64                      `json:"tick"`
		At           time.Time                  `json:"at"`
		CrossSection *CrossSection              `json:"crossSection"`
		Symbols      map[string]any             `json:"symbols"`
		Equity       *kraken.TradeBalanceResult `json:"equity,omitempty"`
	}{
		Status:       thesis.Status,
		Tick:         thesis.Tick,
		At:           thesis.At,
		CrossSection: thesis.CrossSection,
		Symbols:      symbols,
	}

	if hasEquity {
		checkpoint.Equity = &equity
	}

	return json.Marshal(checkpoint)
}

func (thesis *Thesis) Close() error {
	if thesis.cancel != nil {
		thesis.cancel()
	}

	return nil
}
