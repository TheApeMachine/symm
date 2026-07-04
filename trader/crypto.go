package trader

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"github.com/theapemachine/datura"
	"github.com/theapemachine/datura/dmt"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/market"
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
	ctx               context.Context
	cancel            context.CancelFunc
	state             SystemState
	tree              *dmt.Tree
	pool              *qpool.Q[any]
	uiBroadcast       *qpool.BroadcastGroup
	broadcasts        *sync.Map
	subscribers       *sync.Map
	desk              *broker.Desk
	story             *market.Story
	balancesOrigin    string
	balancesScope     string
	balancesType      datura.Artifact_Type
	balancesTimestamp int64
	balancesPayload   []byte
	balancesMu        sync.RWMutex
	signals           *Signal
	crossSection      *market.CrossSection
	allocator         *Allocator
	decider           *Decider
	tick              *atomic.Int64
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
			).With(artifact.Log()...))
		}

		origin := datura.Peek[string](artifact, "origin")
		scope := datura.Peek[string](artifact, "scope")
		payload := append([]byte(nil), artifact.DecryptPayload()...)

		if origin == "" || scope == "" || len(payload) == 0 {
			return errnie.Error(errnie.Err(
				errnie.Validation,
				"trader: balances artifact incomplete",
				nil,
			).With(artifact.Log()...))
		}

		artifactType := artifact.Type()
		if artifactType == 0 {
			artifactType = datura.APPJSON
		}

		balances := datura.Acquire(origin, artifactType).WithRole(role).WithScope(scope).WithPayload(payload)
		balances.SetTimestamp(artifact.Timestamp())
		if len(datura.Peek[[]any](balances, "data")) == 0 {
			return errnie.Error(errnie.Err(
				errnie.Validation,
				"trader: balances snapshot copy missing data",
				nil,
			).With(balances.Log()...))
		}

		crypto.balancesMu.Lock()
		crypto.balancesOrigin = origin
		crypto.balancesScope = scope
		crypto.balancesType = artifactType
		crypto.balancesTimestamp = artifact.Timestamp()
		crypto.balancesPayload = payload
		crypto.balancesMu.Unlock()
		crypto.uiBroadcast.Send(artifact)
	}

	return nil
}

func (crypto *Crypto) Run() error {
	// Empty passes still emit a tick heartbeat so the frontend can distinguish
	// a quiet market from a dead trader loop.
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for crypto.state != READY {
		time.Sleep(1 * time.Second)
		crypto.balancesMu.RLock()
		origin := crypto.balancesOrigin
		scope := crypto.balancesScope
		artifactType := crypto.balancesType
		timestamp := crypto.balancesTimestamp
		payload := append([]byte(nil), crypto.balancesPayload...)
		crypto.balancesMu.RUnlock()

		if origin == "" || scope == "" || len(payload) == 0 {
			continue
		}
		if artifactType == 0 {
			artifactType = datura.APPJSON
		}

		balances := datura.Acquire(origin, artifactType).WithRole("balances").WithScope(scope).WithPayload(payload)
		balances.SetTimestamp(timestamp)
		if len(datura.Peek[[]any](balances, "data")) == 0 {
			continue
		}

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
			"allocator":    crypto.allocator,
			"decider":      crypto.decider,
			"balances":     balances,
		}) == nil {
			crypto.state = READY
		}
	}

	errnie.Info("crypto trader ready")
	lastCandidateCount := 0

	for {
		select {
		case <-crypto.ctx.Done():
			return nil
		case <-ticker.C:
			tickCount := crypto.tick.Add(1)
			crypto.uiBroadcast.Send(datura.Acquire(
				"trader", datura.APPJSON,
			).WithDestination(
				"ui",
			).WithRole(
				"tick",
			).WithScope(
				"tick",
			).WithPayload(datura.Map[any]{
				"count":      tickCount,
				"tick":       tickCount,
				"phase":      "stream",
				"candidates": lastCandidateCount,
				"open":       0,
			}.Marshal()))

			crypto.balancesMu.RLock()
			origin := crypto.balancesOrigin
			scope := crypto.balancesScope
			artifactType := crypto.balancesType
			timestamp := crypto.balancesTimestamp
			payload := append([]byte(nil), crypto.balancesPayload...)
			crypto.balancesMu.RUnlock()

			if origin == "" || scope == "" || len(payload) == 0 {
				return errnie.Error(errnie.Err(
					errnie.Validation,
					"trader: balances artifact unavailable",
					nil,
				))
			}
			if artifactType == 0 {
				artifactType = datura.APPJSON
			}

			balances := datura.Acquire(origin, artifactType).WithRole("balances").WithScope(scope).WithPayload(payload)
			balances.SetTimestamp(timestamp)
			if len(datura.Peek[[]any](balances, "data")) == 0 {
				return errnie.Error(errnie.Err(
					errnie.Validation,
					"trader: balances artifact unavailable",
					nil,
				).With(balances.Log()...))
			}

			measurements := crypto.signals.Measure()
			if len(measurements) == 0 {
				continue
			}
			regime := crypto.signals.Regime()

			crypto.story.Update(measurements)
			actions := crypto.story.Actions(balances)
			lastCandidateCount = len(actions)
			chosen, verdicts := crypto.decider.choose(measurements, actions, balances)
			allowed := crypto.allocator.Allowed(chosen, balances)
			if err := crypto.dispatch(allowed); err != nil {
				return err
			}

			uiMeasurements := map[string]*datura.Artifact{}
			for _, measurement := range measurements {
				measurement.WithAttribute("journey.trader.tick", tickCount)
				uiMeasurements[datura.Peek[string](measurement, "origin")] = measurement
			}

			for _, measurement := range uiMeasurements {
				if measurement == nil {
					continue
				}

				crypto.uiBroadcast.Send(measurement.WithDestination("ui"))
			}
			if regime != nil {
				crypto.uiBroadcast.Send(regime.WithDestination("ui"))
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
		}
	}
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

	return nil
}
