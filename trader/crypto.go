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
	"github.com/theapemachine/symm/logic"
	"github.com/theapemachine/symm/market"
	"github.com/theapemachine/symm/signal/resonance"
)

type SystemState uint8

const (
	PREPARING SystemState = iota
	READY
)

/*
Crypto orchestrates measurement collection, playbook walks, and broker fills.
*/
type Crypto struct {
	ctx          context.Context
	cancel       context.CancelFunc
	state        SystemState
	tree         *dmt.Tree
	pool         *qpool.Q[any]
	uiBroadcast  *qpool.BroadcastGroup
	broadcasts   *sync.Map
	subscribers  *sync.Map
	desk         *broker.Desk
	story        *market.Story
	balances     *datura.Artifact
	signals      *Signal
	crossSection *market.CrossSection
	resonance    *resonance.Signal
	allocator    *Allocator
	decider      *Decider
	tick         *atomic.Int64
}

func NewCrypto(
	ctx context.Context, pool *qpool.Q[any], tree *dmt.Tree,
) (*Crypto, error) {
	ctx, cancel := context.WithCancel(ctx)

	crossSection, err := market.NewCrossSection(
		market.DefaultCrossSectionConfig(),
	)

	if err != nil {
		cancel()
		return nil, errnie.Error(errnie.Err(
			errnie.Validation,
			"trader: failed to create cross section",
			err,
		))
	}

	signals := NewSignal(ctx, pool, tree)

	resonanceSignal := resonance.NewSignal(
		ctx,
		pool,
		tree,
		viper.GetIntSlice("signals.resonance.arch"),
		viper.GetFloat64("signals.resonance.alpha"),
		viper.GetInt("signals.resonance.batch"),
	)

	crypto := &Crypto{
		ctx:          ctx,
		cancel:       cancel,
		state:        PREPARING,
		tree:         tree,
		pool:         pool,
		uiBroadcast:  pool.CreateBroadcastGroup("ui"),
		broadcasts:   &sync.Map{},
		subscribers:  &sync.Map{},
		desk:         broker.NewDesk(ctx, pool, tree),
		story:        market.NewStory(ctx, pool),
		signals:      signals,
		crossSection: crossSection,
		resonance:    resonanceSignal,
		allocator:    NewAllocator(),
		decider:      NewDecider(),
		tick:         &atomic.Int64{},
	}

	for _, channel := range []string{"kraken:public"} {
		crypto.broadcasts.Store(
			channel,
			crypto.pool.CreateBroadcastGroup(channel),
		)
	}

	for _, channel := range []string{"balances"} {
		crypto.subscribers.Store(
			channel, pool.Subscribe(channel, crypto.onMessage),
		)
	}

	return crypto, nil
}

/*
onMessage is called by the qpool.BroadcastGroup for every consumer
that has subscribed with a callback function.
*/
func (crypto *Crypto) onMessage(
	artifact *datura.Artifact,
) error {
	role := datura.Peek[string](artifact, "role")

	switch role {
	case "balances":
		if len(datura.Peek[[]any](artifact, "data")) == 0 {
			return errnie.Error(errnie.Err(
				errnie.Validation,
				"trader: balances artifact missing data",
				nil,
			))
		}

		crypto.balances = artifact
		crypto.uiBroadcast.Send(artifact)
	}

	return nil
}

func (crypto *Crypto) Run() error {
	// The loop is paced by the data, not a fixed wall-clock interval: after each
	// pass it sleeps for the cadence the signals actually observed (bounded).
	// Empty passes still emit a tick heartbeat so the frontend can distinguish
	// a quiet market from a dead trader loop.
	timer := time.NewTimer(crypto.signals.PollInterval())
	defer timer.Stop()

	for crypto.state != READY {
		time.Sleep(1 * time.Second)

		if errnie.Require(map[string]any{
			"ctx":          crypto.ctx,
			"cancel":       crypto.cancel,
			"tree":         crypto.tree,
			"pool":         crypto.pool,
			"uiBroadcast":  crypto.uiBroadcast,
			"broadcasts":   crypto.broadcasts,
			"subscribers":  crypto.subscribers,
			"desk":         crypto.desk,
			"story":        crypto.story,
			"signals":      crypto.signals,
			"crossSection": crypto.crossSection,
			"resonance":    crypto.resonance,
			"allocator":    crypto.allocator,
			"decider":      crypto.decider,
			"balances":     crypto.balances,
		}) == nil {
			crypto.state = READY
		}
	}

	errnie.Info("crypto trader ready")

	for {
		select {
		case <-crypto.ctx.Done():
			return nil
		case <-timer.C:
			timer.Reset(crypto.signals.PollInterval())
			tickCount := crypto.tick.Add(1)

			// Build the peer snapshot once per tick before measuring, so every
			// signal reads a complete, consistent cross-section.
			crypto.signals.Observe(crypto.crossSection)

			measurements := crypto.signals.Measure(crypto.crossSection)
			crypto.story.Update(measurements)
			actions := crypto.story.Actions(crypto.balances)
			chosen, verdicts := crypto.decider.choose(measurements, actions, crypto.balances)
			allowed := crypto.allocator.Allowed(chosen, crypto.balances)

			for _, measurement := range measurements {
				measurement.WithAttribute("journey.trader.tick", tickCount)
				crypto.uiBroadcast.Send(measurement.WithDestination("ui"))
			}

			for _, verdict := range verdicts {
				if verdict.action == nil {
					continue
				}

				crypto.uiBroadcast.Send(verdict.action.WithDestination("ui"))
			}

			for _, action := range allowed {
				if datura.Peek[string](action, "verdict") != "" {
					continue
				}

				crypto.uiBroadcast.Send(action.WithDestination("ui"))
			}

			crypto.uiBroadcast.Send(datura.Acquire(
				"trader", datura.APPJSON,
			).WithDestination(
				"ui",
			).WithRole(
				"tick",
			).WithScope(
				"tick",
			).WithPayload(datura.Map[any]{
				"count":        tickCount,
				"tick":         tickCount,
				"phase":        "stream",
				"candidates":   len(actions),
				"open":         0,
				"quotes_ready": crypto.signals.RoleCount("ticker"),
				"quotes_total": crypto.quotesTotal(measurements),
				"fluid":        originCount(measurements, string(logic.SourceFluid)),
			}.Marshal()))
		}
	}
}

func (crypto *Crypto) quotesTotal(measurements []*datura.Artifact) int {
	if ready := crypto.signals.RoleCount("ticker"); ready > 0 {
		return ready
	}

	scopes := make(map[string]struct{})
	for _, measurement := range measurements {
		scope, err := measurement.Scope()
		if err == nil && scope != "" {
			scopes[scope] = struct{}{}
		}
	}

	return len(scopes)
}

func originCount(measurements []*datura.Artifact, origin string) int {
	count := 0

	for _, measurement := range measurements {
		if measurement == nil {
			continue
		}

		got, err := measurement.Origin()
		if err == nil && got == origin {
			count++
		}
	}

	return count
}

func (crypto *Crypto) Close() error {
	crypto.cancel()

	if crypto.story != nil {
		if err := crypto.story.Close(); err != nil {
			return errnie.Error(err)
		}
	}

	if crypto.signals != nil {
		if err := crypto.signals.Close(); err != nil {
			return errnie.Error(err)
		}
	}

	if crypto.resonance != nil {
		if err := crypto.resonance.Close(); err != nil {
			return errnie.Error(err)
		}
	}

	return nil
}
