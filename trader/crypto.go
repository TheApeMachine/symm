package trader

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"github.com/spf13/viper"
	"github.com/theapemachine/datura"
	"github.com/theapemachine/datura/dmt"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/market"
	"github.com/theapemachine/symm/signal/resonance"
	"github.com/theapemachine/symm/trader/cognitive"
)

/*
Crypto orchestrates measurement collection, playbook walks, and broker fills.
*/
type Crypto struct {
	ctx         context.Context
	cancel      context.CancelFunc
	tree        *dmt.Tree
	pool        *qpool.Q[any]
	subscribers *sync.Map
	desk        *broker.Desk
	story       *market.Story
	resonance   *resonance.Signal
	memory      *cognitive.Memory
	wallet      *datura.Artifact
	storyTicks  atomic.Uint64
}

func NewCrypto(
	ctx context.Context, pool *qpool.Q[any], tree *dmt.Tree,
) *Crypto {
	ctx, cancel := context.WithCancel(ctx)

	crypto := &Crypto{
		ctx:         ctx,
		cancel:      cancel,
		tree:        tree,
		pool:        pool,
		subscribers: &sync.Map{},
		desk:        broker.NewDesk(ctx, pool, tree),
		story:       market.NewStory(ctx, pool),
		resonance: resonance.NewSignal(
			ctx,
			pool,
			tree,
			viper.GetIntSlice("signals.resonance.arch"),
			viper.GetFloat64("signals.resonance.alpha"),
			viper.GetInt("signals.resonance.batch"),
		),
		memory: cognitive.NewMemory(ctx),
	}

	for _, channel := range []string{"balances"} {
		crypto.subscribers.Store(
			channel, pool.Subscribe(channel, crypto.onMessage),
		)
	}

	return crypto
}

func (crypto *Crypto) Run() error {
	crypto.bootstrapWallet()

	interval := viper.GetDuration("market.story.ui_interval")

	if interval <= 0 {
		interval = time.Second
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-crypto.ctx.Done():
			return nil
		case <-ticker.C:
			crypto.measure()
			crypto.applyPlaybookActions()
		}
	}
}

func (crypto *Crypto) onMessage(artifact *datura.Artifact) error {
	role, roleErr := artifact.Role()

	if roleErr != nil {
		return errnie.Error(errnie.Err(
			errnie.Validation,
			"trader: failed to read artifact role",
			roleErr,
		))
	}

	switch role {
	case "balances":
		return crypto.onBalancesMessage(artifact)
	default:
		return nil
	}
}

func (crypto *Crypto) Close() error {
	crypto.cancel()

	if crypto.desk != nil {
		errnie.Error(crypto.desk.Close())
	}

	if crypto.story != nil {
		errnie.Error(crypto.story.Close())
	}

	if crypto.memory != nil {
		errnie.Error(crypto.memory.Close())
	}

	return nil
}
