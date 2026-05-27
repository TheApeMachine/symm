package cmd

import (
	"context"
	"errors"
	"os"
	"sync"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/config"
	"github.com/theapemachine/symm/engine"
	"github.com/theapemachine/symm/ui"
)

type Booter struct {
	ctx     context.Context
	cancel  context.CancelFunc
	err     error
	pool    *qpool.Q
	systems []engine.System
	once    sync.Once
}

func NewBooter(ctx context.Context, pool *qpool.Q) (*Booter, error) {
	ctx, cancel := context.WithCancel(ctx)

	return &Booter{
		ctx:     ctx,
		cancel:  cancel,
		pool:    pool,
		systems: make([]engine.System, 0),
	}, nil
}

func (booter *Booter) AddSystems(systems ...engine.System) {
	booter.systems = append(booter.systems, systems...)

	for _, system := range systems {
		system.Start()
	}
}

func (booter *Booter) Boot() error {
	hub := errnie.Does(func() (*ui.Hub, error) {
		return ui.NewHub(booter.ctx, booter.pool)
	}).Or(func(err error) {
		errnie.Error(err)
	}).Value()

	if hub == nil {
		errnie.Error(errors.New("failed to create ui hub"))
		os.Exit(1)
	}

	go hub.Serve(config.System.UIAddr)

	booter.once.Do(func() {
		for booter.ctx.Err() == nil {
			for _, system := range booter.systems {
				if err := system.Tick(); err != nil {
					errnie.Error(err)
				}
			}
		}
	})

	return booter.err
}
