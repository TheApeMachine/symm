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
	ctx         context.Context
	cancel      context.CancelFunc
	pool        *qpool.Q
	systems     []engine.System
	broadcasts  map[string]*qpool.BroadcastGroup
	subscribers map[string]*qpool.Subscriber
}

func NewBooter(ctx context.Context, pool *qpool.Q) (*Booter, error) {
	ctx, cancel := context.WithCancel(ctx)

	booter := &Booter{
		ctx:     ctx,
		cancel:  cancel,
		pool:    pool,
		systems: make([]engine.System, 0),
	}

	return booter, nil
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

	for _, system := range booter.systems {
		if walletPublisher, ok := system.(interface{ ResendWallet() }); ok {
			walletPublisher.ResendWallet()
		}
	}

	var wg sync.WaitGroup

	for _, system := range booter.systems {
		wg.Go(func() {
			errnie.Error(system.Tick())
		})
	}

	wg.Wait()

	return booter.ctx.Err()
}
