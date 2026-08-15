package types

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	pmanifold "github.com/theapemachine/nomagique/physics/manifold"
	"github.com/theapemachine/symm/kraken"
)

/*
Thesis owns canonical evidence across every evaluated epoch that contributes to
one decision. It closes only after the planner emits the completed decision set;
broker execution and settlement continue in their own lifecycle.
*/
type Thesis struct {
	ctx            context.Context
	cancel         context.CancelFunc
	ui             chan []byte
	equityMu       sync.RWMutex
	equity         *kraken.TradeBalanceResult
	equityRevision uint64
	symbolMu       sync.Mutex
	symbolIDs      map[string]SymbolID
	nextSymbolID   SymbolID
	Status         Status          `json:"status"`
	Tick           int64           `json:"tick"`
	At             time.Time       `json:"at"`
	CrossSection   *CrossSection   `json:"crossSection"`
	Symbols        *sync.Map       `json:"-"`
	Audit          func(any) error `json:"-"`
	manifold       atomic.Pointer[pmanifold.Reading]
	phase          atomic.Pointer[PhaseReading]
}

/*
NewThesis creates a Thesis with empty durable maps and no tick evidence yet.
*/
func NewThesis(
	ctx context.Context, ui chan []byte,
) *Thesis {
	ctx, cancel := context.WithCancel(ctx)

	return &Thesis{
		ctx:          ctx,
		cancel:       cancel,
		ui:           ui,
		Status:       READY,
		CrossSection: NewCrossSection(),
		Symbols:      &sync.Map{},
		symbolIDs:    make(map[string]SymbolID),
	}
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
		created.ID = thesis.nextSymbolID
		thesis.symbolIDs[name] = thesis.nextSymbolID
		thesis.nextSymbolID++
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
		Audit:        thesis.Audit,
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

	for _, name := range names {
		if name == "" {
			return nil, fmt.Errorf("thesis: symbol scope required")
		}

		symbol, found := thesis.Symbols.Load(name)

		if !found {
			return nil, fmt.Errorf("thesis: symbol scope not found: %s", name)
		}

		symbols.Store(name, symbol)
	}

	scoped := &Thesis{
		ctx:          thesis.ctx,
		ui:           thesis.ui,
		Status:       thesis.Status,
		Tick:         thesis.Tick,
		At:           thesis.At,
		CrossSection: thesis.CrossSection,
		Symbols:      symbols,
		Audit:        thesis.Audit,
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
