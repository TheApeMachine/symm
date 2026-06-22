package trader

import (
	"context"
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

	return &Crypto{
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
			grouped := make(map[string][]*datura.Artifact)

			for _, measurement := range crypto.signals.Measure() {
				if measurement == nil {
					continue
				}

				scope := errnie.Does(func() (string, error) {
					return measurement.Scope()
				}).Or(func(err error) {
					errnie.Error(errnie.Err(
						errnie.Validation,
						"trader: measurement scope failed",
						err,
					))
				}).Value()

				grouped[scope] = append(grouped[scope], measurement)
			}

			if len(grouped) > 0 && crypto.resonance != nil {
				scopes := make([]string, 0, len(grouped))

				for scope := range grouped {
					scopes = append(scopes, scope)
				}

				if _, settleErr := crypto.resonance.SettleScopes(scopes); settleErr != nil {
					errnie.Error(errnie.Err(
						errnie.Validation,
						"trader: resonance settle failed",
						settleErr,
					))
				}
			}

			for scope, scopeMeasurements := range grouped {
				crypto.publishMeasurements(scopeMeasurements)
				crypto.evaluateScopeStory(scope, scopeMeasurements)
			}

			crypto.storyTicks.Add(1)

			errnie.Error(crypto.uiBroadcast.Send(
				datura.Acquire("trader", datura.APPJSON).WithPayload(datura.Map[any]{
					"type":        "story_tick",
					"story_ticks": crypto.storyTicks.Load(),
				}.Marshal()).WithDestination(
					"ui",
				).WithRole(
					"story",
				).WithScope(
					"trader",
				),
			))
		}
	}
}

func (crypto *Crypto) publishMeasurements(measurements []*datura.Artifact) {
	for _, measurement := range measurements {
		if measurement == nil {
			continue
		}

		measurement.WithDestination("ui").Inspect(
			"trader", "crypto", "Run()",
		)

		if err := crypto.uiBroadcast.Send(measurement); err != nil {
			errnie.Error(errnie.Err(
				errnie.IO,
				"trader: ui publish failed",
				err,
			))
		}
	}
}

func (crypto *Crypto) evaluateScopeStory(
	scope string,
	measurements []*datura.Artifact,
) {
	if crypto.story == nil || len(measurements) == 0 {
		return
	}

	batch := datura.Acquire("trader-story", datura.APPJSON)
	batch.WithScope(scope)
	batch.Poke(measurements, "measurements")

	verdict := crypto.story.Update(batch)

	batch.Release()

	if verdict == nil {
		return
	}

	verdict.WithDestination("ui")

	if err := crypto.uiBroadcast.Send(verdict); err != nil {
		errnie.Error(errnie.Err(
			errnie.IO,
			"trader: verdict publish failed",
			err,
		))
	}

	if crypto.desk != nil {
		if err := crypto.desk.Update(verdict); err != nil {
			errnie.Error(errnie.Err(
				errnie.IO,
				"trader: desk update failed",
				err,
			))
		}
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

	if crypto.signals != nil {
		errnie.Error(crypto.signals.Close())
	}

	if crypto.resonance != nil {
		errnie.Error(crypto.resonance.Close())
	}

	return nil
}
