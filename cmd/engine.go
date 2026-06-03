package cmd

import (
	"context"
	"errors"
	"fmt"
	"reflect"
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

func (engine *Engine) Start() error {
	var wg sync.WaitGroup
	var errMu sync.Mutex
	tickErrors := make([]error, 0)

	for _, system := range engine.systems {
		system := system

		wg.Go(func() {
			if err := system.Tick(); err != nil {
				errMu.Lock()
				tickErrors = append(tickErrors, err)
				errMu.Unlock()

				return
			}
		})
	}

	wg.Wait()

	engine.err = errors.Join(tickErrors...)

	return engine.err
}

func (engine *Engine) AddSystems(systems ...System) error {
	for _, s := range systems {
		if s == nil || reflect.ValueOf(s).IsNil() {
			return fmt.Errorf("nil system passed to AddSystems")
		}
	}

	engine.systems = append(engine.systems, systems...)

	return nil
}

func (engine *Engine) Close() error {
	engine.cancel()
	return engine.err
}
