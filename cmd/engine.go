package cmd

import (
	"context"
	"errors"
	"sync"

	"github.com/theapemachine/qpool"
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

	return engine, nil
}

func (engine *Engine) Start() (err error) {
	var wg sync.WaitGroup

	for _, system := range engine.systems {
		wg.Go(func() {
			if err = errors.Join(err, system.Tick()); err != nil {
				return
			}
		})
	}

	wg.Wait()
	return err
}

func (engine *Engine) AddSystems(systems ...System) error {
	engine.systems = append(engine.systems, systems...)
	return nil
}

func (engine *Engine) Close() error {
	engine.cancel()
	return engine.err
}
