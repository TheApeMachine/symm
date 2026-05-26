package cmd

import (
	"context"
	"errors"
	"os"
	"sync"
	"time"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/config"
	"github.com/theapemachine/symm/engine"
	"github.com/theapemachine/symm/ui"
)

type Booter struct {
	ctx         context.Context
	cancel      context.CancelFunc
	err         error
	pool        *qpool.Q
	systems     []engine.System
	broadcasts  map[string]*qpool.BroadcastGroup
	subscribers map[string]*qpool.Subscriber
	once        sync.Once
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

	booter.once.Do(func() {
		for {
			select {
			case <-booter.ctx.Done():
				booter.cancel()
				booter.err = errnie.Error(booter.ctx.Err())
				return
			default:
				for _, system := range booter.systems {
					if system.State() != engine.READY {
						continue
					}

					if err := system.Tick(); err != nil {
						booter.err = errnie.Error(err)
					}
				}

				if delay := config.System.RescoreEvery; delay > 0 {
					time.Sleep(delay)
				}
			}
		}
	})

	return booter.err
}
