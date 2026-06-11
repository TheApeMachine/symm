package cmd

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"slices"
	"sync"
	"time"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/internal"
	"github.com/theapemachine/symm/kraken/market"
	"github.com/theapemachine/symm/kraken/types"
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
			[]internal.Channel{internal.ChannelKrakenPublic},
			nil,
		),
		systems: make([]System, 0),
	}

	return engine, nil
}

func (engine *Engine) Context() context.Context {
	return engine.ctx
}

func (engine *Engine) Start() error {
	var (
		waitGroup sync.WaitGroup
	)

	hub := ui.NewHub(engine.ctx, engine.pool)
	defer hub.Close()

	for _, system := range engine.systems {
		waitGroup.Go(func() {
			if err := system.Tick(); err != nil && !internal.IsShutdown(err) {
				errnie.Error(err)
				engine.cancel()
			}
		})
	}

	if err := errnie.Error(engine.bus.Send(
		internal.ChannelKrakenPublic,
		"instrument",
		types.KrakenMessage{
			Method: "subscribe",
			Params: market.InstrumentParams{
				Channel:  "instrument",
				Snapshot: true,
			},
			ReqID: time.Now().UnixNano(),
		},
	)); err != nil {
		engine.cancel()
		return err
	}

	waitGroup.Wait()
	return errnie.Error(engine.Close())
}

func (engine *Engine) AddSystems(systems ...System) error {
	if slices.ContainsFunc(systems, systemNil) {
		return fmt.Errorf("engine: nil system")
	}

	engine.systems = append(engine.systems, systems...)

	return nil
}

func systemNil(system System) bool {
	if system == nil {
		return true
	}

	value := reflect.ValueOf(system)

	return value.Kind() == reflect.Pointer && value.IsNil()
}

func (engine *Engine) Close() (err error) {
	engine.cancel()

	for _, system := range engine.systems {
		if closeErr := system.Close(); closeErr != nil {
			err = errors.Join(err, fmt.Errorf("%T: %w", system, closeErr))
		}
	}

	return errnie.Error(err)
}
