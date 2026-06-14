package causal

import (
	"context"
	"sync"
	"time"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/nomagique/correlation"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/logic"
	"github.com/theapemachine/symm/market"
	"github.com/theapemachine/symm/signal"
)

/*
System owns per-symbol causal state and cross-asset contagion estimation.
*/
type System struct {
	base               *signal.System
	symbols            sync.Map
	contagionEstimator *correlation.Contagion
	contagionCache     float64
	contagionAt        time.Time
	lastPublish        time.Time
}

func NewSystem(ctx context.Context, pool *qpool.Q[any]) *System {
	system := &System{}

	base := signal.NewSystem(
		ctx,
		pool,
		logic.SourceCausal,
		func(symbol string) market.Signal {
			return NewSignal(symbol, nil, system)
		},
	)

	if base == nil {
		return nil
	}

	system.base = base

	return system
}

func (system *System) loadSymbol(symbol string) (*CausalSymbol, error) {
	raw, _ := system.symbols.LoadOrStore(symbol, &symbolSlot{})

	slot, ok := raw.(*symbolSlot)

	if !ok {
		return nil, errnie.Error(errnie.Err(
			errnie.IO,
			"causal: invalid symbol slot",
			nil,
		))
	}

	return slot.load()
}

func (system *System) shouldPublish(at time.Time) bool {
	interval := loadRuntimeConfig().PublishInterval

	if interval <= 0 {
		return true
	}

	if system.lastPublish.IsZero() || at.Sub(system.lastPublish) >= interval {
		system.lastPublish = at
		return true
	}

	return false
}

func (system *System) Tick() error {
	return system.base.Tick()
}

func (system *System) Close() error {
	return system.base.Close()
}

type symbolSlot struct {
	once   sync.Once
	symbol *CausalSymbol
	err    error
}

func (slot *symbolSlot) load() (*CausalSymbol, error) {
	slot.once.Do(func() {
		slot.symbol, slot.err = NewCausalSymbol()
	})

	return slot.symbol, slot.err
}
