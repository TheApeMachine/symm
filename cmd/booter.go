package cmd

import (
	"context"
	"errors"
	"os"
	"sync"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/config"
	"github.com/theapemachine/symm/ui"
)

type System interface {
	Tick() error
	Close() error
}

type Booter struct {
	ctx         context.Context
	cancel      context.CancelFunc
	pool        *qpool.Q
	systems     []System
	broadcasts  map[string]*qpool.BroadcastGroup
	subscribers map[string]*qpool.Subscriber
}

func NewBooter(ctx context.Context, pool *qpool.Q) (*Booter, error) {
	ctx, cancel := context.WithCancel(ctx)

	booter := &Booter{
		ctx:     ctx,
		cancel:  cancel,
		pool:    pool,
		systems: make([]System, 0),
	}

	return booter, nil
}

func (booter *Booter) AddSystems(systems ...System) error {
	booter.systems = append(booter.systems, systems...)
	return nil
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
	var shutdown sync.Once

	closeAll := func() {
		shutdown.Do(func() {
			booter.cancel()

			// Systems hold contexts that are siblings (not children) of
			// booter.ctx, so cancelling the booter is not enough to stop them.
			// Call Close on each so its internal cancel runs.
			for _, system := range booter.systems {
				if err := system.Close(); err != nil {
					errnie.Error(err)
				}
			}
		})
	}

	for _, system := range booter.systems {
		wg.Go(func() {
			// If any system's Tick exits (error or clean return), tear down
			// the remaining systems rather than letting them spin on stale
			// data from the failed peer.
			if err := system.Tick(); err != nil {
				errnie.Error(err)
			}
			closeAll()
		})
	}

	wg.Wait()

	return booter.ctx.Err()
}
