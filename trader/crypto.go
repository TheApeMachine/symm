package trader

import (
	"context"
	"strings"
	"sync/atomic"
	"time"

	"github.com/spf13/viper"
	"github.com/theapemachine/datura"
	"github.com/theapemachine/datura/dmt"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/logic"
	"github.com/theapemachine/symm/market"
	"github.com/theapemachine/symm/signal/resonance"
	"github.com/theapemachine/symm/ui"
)

/*
Crypto orchestrates measurement collection, playbook walks, and broker fills.
*/
type Crypto struct {
	ctx         context.Context
	cancel      context.CancelFunc
	tree        *dmt.Tree
	pool        *qpool.Q[any]
	uiBroadcast *qpool.BroadcastGroup
	desk        *broker.Desk
	story       *market.Story
	signals     *Signal
	resonance   *resonance.Signal
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
		uiBroadcast: pool.CreateBroadcastGroup("ui"),
		desk:        broker.NewDesk(ctx, pool, tree),
		story:       market.NewStory(ctx, pool),
		signals:     NewSignal(ctx, pool, tree),
		resonance: resonance.NewSignal(
			ctx,
			pool,
			tree,
			viper.GetIntSlice("signals.resonance.arch"),
			viper.GetFloat64("signals.resonance.alpha"),
			viper.GetInt("signals.resonance.batch"),
		),
	}

	return crypto
}

func (crypto *Crypto) Run() error {
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
			crypto.publishStateFrame()
		}
	}
}

func (crypto *Crypto) publishStateFrame() {
	scope := measurementScope()

	if scope == "" {
		return
	}

	measurements := crypto.signals.Measure(scope)

	if crypto.story == nil || crypto.story.PlaybookTree() == nil {
		return
	}

	walk := logic.WalkTree(
		scope,
		measurements,
		&logic.Balances{},
		crypto.story.PlaybookTree().Branches,
	)

	storyTicks := crypto.storyTicks.Add(1)
	frame := ui.StateFrame(measurements, storyTicks, &walk)

	artifact := datura.Acquire("trader", datura.APPJSON)
	artifact.WithRole("state")
	artifact.WithScope(scope)

	if err := artifact.From(frame); err != nil {
		errnie.Error(errnie.Err(
			errnie.Validation,
			"trader: failed to marshal state frame",
			err,
		))

		artifact.Release()

		return
	}

	crypto.uiBroadcast.Send(artifact)
}

func measurementScope() string {
	anchor := strings.TrimSpace(viper.GetString("market.anchor_symbol"))

	if anchor != "" {
		return anchor
	}

	for _, scope := range viper.GetStringSlice("market.default_symbols") {
		scope = strings.TrimSpace(scope)

		if scope != "" {
			return scope
		}
	}

	return ""
}

func (crypto *Crypto) Close() error {
	crypto.cancel()

	if crypto.desk != nil {
		errnie.Error(crypto.desk.Close())
	}

	if crypto.story != nil {
		errnie.Error(crypto.story.Close())
	}

	if crypto.signals != nil {
		errnie.Error(crypto.signals.Close())
	}

	if crypto.resonance != nil {
		errnie.Error(crypto.resonance.Close())
	}

	return nil
}
