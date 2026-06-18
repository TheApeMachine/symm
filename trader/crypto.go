package trader

import (
	"context"
	"sync"
	"time"

	"github.com/spf13/viper"
	"github.com/theapemachine/datura"
	"github.com/theapemachine/datura/dmt"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/signal/resonance"
)

/*
surpriseStats tracks running mean and variance for resonance surprise gating.
*/
type surpriseStats struct {
	mean float64
	m2   float64
	n    float64
}

/*
Crypto is a trader that is responsible for orchestrating
the trading of crypto assets. It should collect the data
it needs to make informed decisions regarding the opening,
closing, and reporting of positions.
*/
type Crypto struct {
	ctx        context.Context
	cancel     context.CancelFunc
	err        error
	tree       *dmt.Tree
	pool       *qpool.Q[any]
	broadcasts *sync.Map
	desk       *broker.Desk
	resonance  *resonance.Signal
}

func NewCrypto(
	ctx context.Context, pool *qpool.Q[any],
) *Crypto {
	ctx, cancel := context.WithCancel(ctx)

	crypto := &Crypto{
		ctx:        ctx,
		cancel:     cancel,
		tree:       dmt.NewTree(""),
		pool:       pool,
		broadcasts: &sync.Map{},
		desk:       broker.NewDesk(ctx, pool),
	}

	return crypto
}

func (crypto *Crypto) Run() error {
	interval := viper.GetDuration("market.story.ui_interval")

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-crypto.ctx.Done():
			return nil
		case <-ticker.C:
			measurement := crypto.resonance.Measure(
				datura.Acquire(
					"trader", datura.APPJSON,
				).WithRole(
					"measurement",
				).WithScope(
					"resonance",
				),
			)
		}
	}
}
