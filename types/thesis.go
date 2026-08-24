package types

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/theapemachine/symm/kraken"
	pmanifold "github.com/theapemachine/symm/nomagique/physics/sensorium"
)

/*
Thesis is the pure shared state of the running system: the symbol universe,
the engine clock, the cross-section, and the latest fluid readings. It owns
no pipeline machinery — no queues, no work scheduling, no bus. Market data
flows over the runtime workspace bus; everything here is state that stages
read and write.
*/
type Thesis struct {
	ctx            context.Context
	cancel         context.CancelFunc
	balance        kraken.BalanceData
	equityMu       sync.RWMutex
	equity         *kraken.TradeBalanceResult
	equityRevision uint64
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
	interventions  atomic.Pointer[[]InterventionScore]
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
AdvanceTick commits one real market observation to the engine clock.
*/
func (thesis *Thesis) AdvanceTick(at time.Time) int64 {
	thesis.Tick++
	thesis.At = at

	return thesis.Tick
}

/*
NewThesis creates a Thesis with empty durable state and no tick evidence yet.
*/
func NewThesis(ctx context.Context) *Thesis {
	ctx, cancel := context.WithCancel(ctx)

	return &Thesis{
		ctx:          ctx,
		cancel:       cancel,
		Status:       READY,
		CrossSection: NewCrossSection(),
		Symbols:      &sync.Map{},
	}
}

/*
Symbol returns the shared state for a symbol, creating it on first use.
*/
func (thesis *Thesis) Symbol(name string) *Symbol {
	symbol, ok := thesis.Symbols.Load(name)

	if ok {
		return symbol.(*Symbol)
	}

	thesis.symbolMu.Lock()
	defer thesis.symbolMu.Unlock()

	symbol, ok = thesis.Symbols.Load(name)

	if !ok {
		created := NewSymbol(name)
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
symbol, sharing the cross-section and fluid readings with its parent.
*/
func (thesis *Thesis) ForSymbol(name string) (*Thesis, error) {
	if thesis == nil || name == "" {
		return nil, fmt.Errorf("thesis: symbol scope required")
	}

	symbol, found := thesis.Symbols.Load(name)

	if !found {
		return nil, fmt.Errorf("thesis: symbol scope not found: %s", name)
	}

	return thesis.scoped([]*Symbol{symbol.(*Symbol)}), nil
}

/*
ForSymbols returns a non-owning analytical view containing exactly the named
existing symbols.
*/
func (thesis *Thesis) ForSymbols(names []string) (*Thesis, error) {
	if thesis == nil || len(names) == 0 {
		return nil, fmt.Errorf("thesis: symbol scope required")
	}

	ordered := make([]*Symbol, 0, len(names))

	for _, name := range names {
		if name == "" {
			return nil, fmt.Errorf("thesis: symbol scope required")
		}

		symbol, found := thesis.Symbols.Load(name)

		if !found {
			return nil, fmt.Errorf("thesis: symbol scope not found: %s", name)
		}

		ordered = append(ordered, symbol.(*Symbol))
	}

	return thesis.scoped(ordered), nil
}

func (thesis *Thesis) scoped(symbols []*Symbol) *Thesis {
	scopedSymbols := &sync.Map{}

	for _, symbol := range symbols {
		scopedSymbols.Store(symbol.Symbol, symbol)
	}

	scoped := &Thesis{
		ctx:          thesis.ctx,
		Status:       thesis.Status,
		Tick:         thesis.Tick,
		At:           thesis.At,
		CrossSection: thesis.CrossSection,
		Symbols:      scopedSymbols,
		symbols:      symbols,
		Audit:        thesis.Audit,
	}

	if manifold := thesis.manifold.Load(); manifold != nil {
		scoped.manifold.Store(manifold)
	}

	if phase := thesis.phase.Load(); phase != nil {
		scoped.phase.Store(phase)
	}

	if interventions := thesis.interventions.Load(); interventions != nil {
		scoped.interventions.Store(interventions)
	}

	return scoped
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
InterventionScore is the crystallization diagnostic of one do(X) BVP probe.
*/
type InterventionScore struct {
	Action            string  `json:"action"`
	Score             float64 `json:"score"`
	MassGain          float64 `json:"massGain"`
	EnergyGain        float64 `json:"energyGain"`
	HeatShock         float64 `json:"heatShock"`
	SpectralResonance float64 `json:"spectralResonance"`
}

/*
StoreInterventions publishes the latest manifold do(X) crystallization scores.
*/
func (thesis *Thesis) StoreInterventions(scores []InterventionScore) {
	copied := append([]InterventionScore(nil), scores...)
	thesis.interventions.Store(&copied)
}

/*
InterventionSnapshot returns the latest do(X) crystallization scores.
*/
func (thesis *Thesis) InterventionSnapshot() ([]InterventionScore, bool) {
	scores := thesis.interventions.Load()

	if scores == nil {
		return nil, false
	}

	return *scores, true
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

		symbols[name] = symbol
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
