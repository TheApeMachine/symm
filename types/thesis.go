package types

import (
	"context"
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
	ctx          context.Context
	cancel       context.CancelFunc
	ui           chan []byte
	equityMu     sync.RWMutex
	equity       *kraken.TradeBalanceResult
	Status       Status          `json:"status"`
	Tick         int64           `json:"tick"`
	At           time.Time       `json:"at"`
	CrossSection *CrossSection   `json:"crossSection"`
	Symbols      *sync.Map       `json:"-"`
	Audit        func(any) error `json:"-"`
	Manifold     fluid.Reading   `json:"-"`
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

func (thesis *Thesis) Close() error {
	if thesis.cancel != nil {
		thesis.cancel()
	}

	return nil
}
