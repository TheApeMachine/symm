package cmd

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/internal"
	"github.com/theapemachine/symm/kraken/market"
	"github.com/theapemachine/symm/kraken/types"
)

type System interface {
	Tick() error
	Close() error
}

type Engine struct {
	ctx     context.Context
	cancel  context.CancelFunc
	err     error
	pool    *qpool.Q[any]
	bus     *internal.Bus
	systems []System
}

func NewEngine(ctx context.Context, pool *qpool.Q[any]) (*Engine, error) {
	ctx, cancel := context.WithCancel(ctx)

	engine := &Engine{
		ctx:    ctx,
		cancel: cancel,
		pool:   pool,
		bus: internal.NewBus(
			ctx,
			pool,
			[]string{"kraken:public"},
			[]string{},
		),
		systems: make([]System, 0),
	}

	return engine, nil
}

func (engine *Engine) Context() context.Context {
	return engine.ctx
}

func (engine *Engine) Start() (err error) {
	wg := sync.WaitGroup{}

	for _, system := range engine.systems {
		wg.Go(func() {
			if err := system.Tick(); err != nil {
				errnie.Error(err, "%T: %w", system, err)
			}
		})
	}

	engine.bus.Send("kraken:public", "instrument", types.KrakenMessage{
		Method: "subscribe",
		Params: market.NewInstrumentParams(),
		ReqID: time.Now().UnixNano(),
	})

	wg.Wait()
	return nil
}

func (engine *Engine) AddSystems(systems ...System) error {
	for _, system := range systems {
		if system == nil {
			return fmt.Errorf("engine: nil system")
		}
	}

	engine.systems = append(engine.systems, systems...)
	return nil
}

func (engine *Engine) Close() (err error) {
	engine.cancel()

	for _, system := range engine.systems {
		if err := system.Close(); err != nil {
			err = errors.Join(err, fmt.Errorf("%T: %w", system, err))
		}
	}

	return errnie.Error(err)
}
