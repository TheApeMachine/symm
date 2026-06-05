package cmd

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/theapemachine/errnie"
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

func (engine *Engine) Context() context.Context {
	return engine.ctx
}

func (engine *Engine) Start() (err error) {
	var wg sync.WaitGroup
	errs := make(chan error, len(engine.systems))

	for _, system := range engine.systems {
		system := system

		wg.Go(func() {
			errs <- system.Tick()
		})
	}

	go func() {
		wg.Wait()
		close(errs)
	}()

	closing := false

	for tickErr := range errs {
		if tickErr == nil {
			continue
		}

		if engine.ctx.Err() != nil && errors.Is(tickErr, engine.ctx.Err()) {
			if !closing {
				closing = true
				if closeErr := engine.Close(); closeErr != nil {
					engine.err = errors.Join(engine.err, closeErr)
				}
			}

			continue
		}

		if engine.err == nil {
			engine.err = tickErr
			errnie.Error(tickErr)
		}

		if closing {
			continue
		}

		closing = true
		if closeErr := engine.Close(); closeErr != nil && !errors.Is(closeErr, tickErr) {
			engine.err = errors.Join(engine.err, closeErr)
		}
	}

	wg.Wait()

	return engine.err
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

func (engine *Engine) Close() error {
	engine.cancel()

	var closeErr error

	for _, system := range engine.systems {
		if system == nil {
			continue
		}

		if err := system.Close(); err != nil {
			closeErr = errors.Join(closeErr, fmt.Errorf("%T: %w", system, err))
		}
	}

	return errors.Join(engine.err, closeErr)
}
