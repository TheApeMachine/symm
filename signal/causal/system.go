package causal

import (
	"context"
	"sync"
	"time"

	"github.com/spf13/viper"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/qpool"
	krakenmarket "github.com/theapemachine/symm/kraken/market"
	"github.com/theapemachine/symm/logic"
	"github.com/theapemachine/symm/market"
	symmring "github.com/theapemachine/symm/ring"
	"github.com/theapemachine/symm/signal"
)

var (
	sectionLoader market.CrossSectionOnce
	crossSection  *market.CrossSection
)

type System struct {
	base            *signal.System
	symbols         sync.Map
	contagionSpread symmring.FloatRing
	lastPublish     time.Time
}

func NewSystem(ctx context.Context, pool *qpool.Q[any]) *System {
	var err error

	crossSection, err = market.LoadCrossSection(&sectionLoader)

	if errnie.Error(err) != nil {
		return nil
	}

	system := &System{}

	base := signal.NewSystem(
		ctx,
		pool,
		logic.SourceCausal,
		func(symbol string, entity *logic.Entity) market.Signal {
			return NewSignal(symbol, entity, system)
		},
	)

	if base == nil {
		return nil
	}

	system.base = base

	return system
}

func (system *System) Tick() error {
	return system.base.Tick()
}

func (system *System) Close() error {
	return system.base.Close()
}

func (system *System) loadSymbol(symbol string) *CausalSymbol {
	raw, _ := system.symbols.LoadOrStore(symbol, NewCausalSymbol())

	return raw.(*CausalSymbol)
}

func (system *System) shouldPublish(now time.Time) bool {
	interval := viper.GetDuration("signals.causal.publish_interval")

	if interval <= 0 {
		return false
	}

	if now.Sub(system.lastPublish) < interval {
		return false
	}

	system.lastPublish = now

	return true
}

func (system *System) observeTicker(ticker *krakenmarket.TickerUpdate, at time.Time) error {
	row, err := ticker.CompleteSymbol(at, 1)

	if err != nil {
		return err
	}

	return crossSection.Observe(row)
}
