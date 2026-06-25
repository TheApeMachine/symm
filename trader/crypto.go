package trader

import (
	"context"
	"sync"
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
	ctx          context.Context
	cancel       context.CancelFunc
	tree         *dmt.Tree
	pool         *qpool.Q[any]
	uiBroadcast  *qpool.BroadcastGroup
	broadcasts   *sync.Map
	desk         *broker.Desk
	story        *market.Story
	signals      *Signal
	crossSection *market.CrossSection
	resonance    *resonance.Signal
	decider      *Decider
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
		tree:         tree,
		pool:         pool,
		uiBroadcast:  pool.CreateBroadcastGroup("ui"),
		broadcasts:   &sync.Map{},
		desk:         broker.NewDesk(ctx, pool, tree),
		story:        market.NewStory(ctx, pool),
		signals:      signals,
		crossSection: crossSection,
		resonance:    resonanceSignal,
		decider:      NewDecider(),
	}

	for _, channel := range []string{"kraken:public"} {
		crypto.broadcasts.Store(
			channel,
			crypto.pool.CreateBroadcastGroup(channel),
		)
	}

	return crypto, nil
}

func (crypto *Crypto) Run() error {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-crypto.ctx.Done():
			return nil
		case <-ticker.C:
			measurements := crypto.signals.Measure(crypto.crossSection)
			measurements = append(measurements, crypto.resonate(measurements)...)

			// Holdings come from the tree (paper and live both publish balances
			// there via frame.Publish), so the playbook and decider evaluate
			// against the live, fill-mutated ledger.
			balances := holdings(crypto.tree)
			actions := crypto.story.Update(measurements, balances)

			// The decider ranks candidates by expected free energy against the
			// manifold field and dispatches only positive-edge entries the ledger
			// does not already hold (plus all protective exits). The single
			// decision point.
			chosen := crypto.decider.choose(measurements, actions, balances)
			errnie.Error(crypto.desk.Update(chosen))

			for _, measurement := range measurements {
				crypto.uiBroadcast.Send(
					measurement.WithDestination("ui"),
				)
			}
		}
	}
}

/*
resonate settles the resonance batch for the symbols present in this tick's
measurements and returns the resonance measurement artifacts. These carry the
per-symbol reconstruction surprise the decider folds into entry precision.
Symbols resonance has no data for yield no measurement (precision stays unit).
*/
func (crypto *Crypto) resonate(
	measurements []*datura.Artifact,
) []*datura.Artifact {
	if crypto.resonance == nil || len(measurements) == 0 {
		return nil
	}

	seen := make(map[string]struct{})
	scopes := make([]string, 0, len(measurements))

	for _, measurement := range measurements {
		scope := errnie.Does(func() (string, error) {
			return measurement.Scope()
		}).Or(func(err error) {
			errnie.Error(errnie.Err(
				errnie.Validation,
				"crypto: failed to get measurement scope",
				err,
			))
		}).Value()

		if scope == "" {
			continue
		}

		if _, ok := seen[scope]; ok {
			continue
		}

		seen[scope] = struct{}{}
		scopes = append(scopes, scope)
	}

	if len(scopes) == 0 {
		return nil
	}

	settled := errnie.Does(func() (map[string]*datura.Artifact, error) {
		return crypto.resonance.SettleScopes(scopes)
	}).Or(func(err error) {
		errnie.Error(errnie.Err(
			errnie.Validation,
			"crypto: failed to settle resonance scopes",
			err,
		))
	}).Value()

	resonances := make([]*datura.Artifact, 0, len(settled))

	for _, measurement := range settled {
		if measurement != nil {
			resonances = append(resonances, measurement)
		}
	}

	return resonances
}

func (crypto *Crypto) Close() error {
	crypto.cancel()

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
