package types

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/theapemachine/nomagique/physics/fluid"
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
	Status         Status          `json:"status"`
	Tick           int64           `json:"tick"`
	At             time.Time       `json:"at"`
	CrossSection   *CrossSection   `json:"crossSection"`
	Symbols        *sync.Map       `json:"-"`
	Audit          func(any) error `json:"-"`
	Manifold       fluid.Reading   `json:"-"`
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
		At:           time.Now().UTC(),
		CrossSection: NewCrossSection(),
		Symbols:      &sync.Map{},
		Manifold:     fluid.Reading{},
	}
}

func (thesis *Thesis) Symbol(name string) *Symbol {
	symbol, ok := thesis.Symbols.Load(name)

	if !ok {
		symbol = NewSymbol(name, thesis.ui)
		thesis.Symbols.Store(name, symbol)
	}

	return symbol.(*Symbol)
}

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
	return json.Marshal(thesis)
}

func (thesis *Thesis) Close() error {
	if thesis.cancel != nil {
		thesis.cancel()
	}

	return nil
}
