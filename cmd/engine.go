package cmd

import (
	"context"
	"errors"
	"sync"

	"github.com/spf13/viper"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/ui"
)

type System interface {
	Tick() error
	Close() error
}

type Engine struct {
	ctx     context.Context
	cancel  context.CancelFunc
	err     error
	pool    *qpool.Q
	systems []System
}

func NewEngine(ctx context.Context, pool *qpool.Q) (*Engine, error) {
	ctx, cancel := context.WithCancel(ctx)

	engine := &Engine{
		ctx:     ctx,
		cancel:  cancel,
		pool:    pool,
		systems: make([]System, 0),
	}

	return engine, errnie.Error(errnie.Require(map[string]any{
		"ctx":     engine.ctx,
		"cancel":  engine.cancel,
		"pool":    engine.pool,
		"systems": engine.systems,
	}))
}

func (engine *Engine) Start() error {
	var wg sync.WaitGroup

	hub := errnie.Does(func() (*ui.Hub, error) {
		return ui.NewHub(engine.ctx, engine.pool)
	}).Or(func(err error) {
		errnie.Error(err)
	}).Value()

	go hub.Serve(viper.GetViper().GetString("ui.addr"))

	for _, system := range engine.systems {
		wg.Go(func() {
			if err := system.Tick(); err != nil {
				engine.err = errors.Join(engine.err, errnie.Error(err))
			}
		})
	}

	wg.Wait()

	return engine.err
}

func (engine *Engine) AddSystems(systems ...System) error {
	engine.systems = append(engine.systems, systems...)
	return nil
}

func (engine *Engine) Close() error {
	engine.cancel()
	return engine.err
}
