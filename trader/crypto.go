package trader

import (
	"context"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/bytedance/sonic"
	"github.com/spf13/viper"
	"github.com/theapemachine/datura"
	"github.com/theapemachine/datura/dmt"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/logic"
	"github.com/theapemachine/symm/market"
	"github.com/theapemachine/symm/signal/manifold"
	"github.com/theapemachine/symm/signal/resonance"
)

/*
Crypto orchestrates measurement collection, playbook walks, and broker fills.
*/
type Crypto struct {
	ctx           context.Context
	cancel        context.CancelFunc
	tree          *dmt.Tree
	pool          *qpool.Q[any]
	uiBroadcast   *qpool.BroadcastGroup
	broadcasts    *sync.Map
	desk          *broker.Desk
	story         *market.Story
	signals       *Signal
	crossSection  *market.CrossSection
	resonance     *resonance.Signal
	decider       *Decider
	tick          *atomic.Int64
	quoteCurrency string
	tradingModel  string
}

func openHoldingCount(balances *logic.Balances, quoteCurrency string) int {
	if balances == nil {
		return 0
	}

	count := 0

	for _, asset := range balances.Asset {
		if asset.Balance > 0 && strings.ToUpper(asset.Asset) != quoteCurrency {
			count++
		}
	}

	return count
}

func measurementOriginCounts(measurements []*datura.Artifact) map[string]int {
	counts := make(map[string]int)

	for _, measurement := range measurements {
		origin, err := measurement.Origin()

		if err != nil || origin == "" {
			continue
		}

		counts[origin]++
	}

	return counts
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
		ctx:           ctx,
		cancel:        cancel,
		tree:          tree,
		pool:          pool,
		uiBroadcast:   pool.CreateBroadcastGroup("ui"),
		broadcasts:    &sync.Map{},
		desk:          broker.NewDesk(ctx, pool, tree),
		story:         market.NewStory(ctx, pool),
		signals:       signals,
		crossSection:  crossSection,
		resonance:     resonanceSignal,
		decider:       NewDecider(),
		tick:          &atomic.Int64{},
		quoteCurrency: strings.ToUpper(viper.GetString("market.quote_currency")),
		tradingModel:  viper.GetString("trading.model"),
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
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-crypto.ctx.Done():
			return nil
		case <-ticker.C:
			// Build the peer snapshot once per tick before measuring, so every
			// signal reads a complete, consistent cross-section.
			crypto.signals.Observe(crypto.crossSection)

			measurements := crypto.signals.Measure(crypto.crossSection)
			measurements = append(measurements, crypto.resonate(measurements)...)

			// Holdings come from the tree (paper and live both publish balances
			// there via frame.Publish), so the playbook and decider evaluate
			// against the live, fill-mutated ledger.
			balances := holdings(crypto.tree)
			actions := crypto.story.Update(measurements, balances)
			originCounts := measurementOriginCounts(measurements)

			// The decider ranks candidates by expected free energy against the
			// manifold field and dispatches only positive-edge entries the ledger
			// does not already hold (plus all protective exits). The single
			// decision point.
			chosen, rejections := crypto.decider.choose(measurements, actions, balances)
			errnie.Error(crypto.desk.Update(chosen))

			rejectionReason := make(map[string]string, len(rejections))
			for _, rejection := range rejections {
				rejectionReason[rejection.symbol] = rejection.reason
			}

			type chosenKey struct {
				symbol     string
				side       string
				actionType string
			}

			chosenMap := make(map[chosenKey]struct{}, len(chosen))
			for _, choice := range chosen {
				choiceSymbol, _ := choice.Scope()
				choiceSide, _ := choice.Role()
				choiceType := datura.Peek[string](choice, "type")
				chosenMap[chosenKey{
					symbol:     choiceSymbol,
					side:       choiceSide,
					actionType: choiceType,
				}] = struct{}{}
			}

			type decisionUI struct {
				Symbol     string  `json:"symbol"`
				Side       string  `json:"side"`
				Type       string  `json:"type"`
				Price      float64 `json:"price"`
				Quantity   float64 `json:"quantity"`
				Verdict    string  `json:"verdict"`
				Why        string  `json:"why"`
				Confidence float64 `json:"confidence"`
			}

			uiDecisions := make([]decisionUI, 0, len(actions))

			for _, action := range actions {
				symbol, _ := action.Scope()
				side, _ := action.Role()
				actionType := datura.Peek[string](action, "type")
				price := datura.Peek[float64](action, "price")
				qty := datura.Peek[float64](action, "quantity")
				confidence := datura.Peek[float64](action, "entry_confidence")

				verdict := "blocked"

				// The reason is the decider's recorded verdict for this symbol —
				// missing data source, field veto, no slot — not a fabricated
				// default. An admitted entry overrides to allow.
				why, hasReason := rejectionReason[symbol]
				if !hasReason {
					why = "below edge"
				}

				_, isChosen := chosenMap[chosenKey{
					symbol:     symbol,
					side:       side,
					actionType: actionType,
				}]
				if isChosen {
					verdict = "allow"
					why = "admitted"
				}

				action.Inspect("crypto", "Run", "actions")

				uiDecisions = append(uiDecisions, decisionUI{
					Symbol:     symbol,
					Side:       side,
					Type:       actionType,
					Price:      price,
					Quantity:   qty,
					Verdict:    verdict,
					Why:        why,
					Confidence: confidence,
				})
			}

			payloadMap := map[string]any{
				"decisions": uiDecisions,
			}

			if marshaled, err := sonic.Marshal(payloadMap); err == nil {
				decisionsArtifact := datura.Acquire("trader-decisions", datura.APPJSON).
					WithDestination("ui").
					WithRole("decisions").
					WithScope("decisions").
					WithPayload(marshaled)
				crypto.uiBroadcast.Send(decisionsArtifact)
			}

			// Publish the per-symbol playbook descent traces so the Decision Tree
			// surface renders the real walk: which branches matched, parked, or
			// rejected, keyed by symbol (the shape the frontend evaluations
			// consumer expects).
			if traces := crypto.story.Traces(); len(traces) > 0 {
				evaluations := make(map[string]logic.WalkTrace, len(traces))

				for _, trace := range traces {
					evaluations[trace.Symbol] = trace
				}

				if marshaled, err := sonic.Marshal(map[string]any{"evaluations": evaluations}); err == nil {
					walkArtifact := datura.Acquire("trader-walk", datura.APPJSON).
						WithDestination("ui").
						WithRole("walk").
						WithScope("walk").
						WithPayload(marshaled)
					crypto.uiBroadcast.Send(walkArtifact)
				}
			}

			// Derive and publish per-symbol cognitive readings (regime sequence,
			// entropy gate, winner class) from this tick's measurement set so the
			// Cortex surface renders real cognitive state, not a simulated beam.
			if cognitive := market.CognitiveReadings(crypto.tree, measurements); len(cognitive) > 0 {
				if marshaled, err := sonic.Marshal(map[string]any{"readings": cognitive}); err == nil {
					cognitiveArtifact := datura.Acquire("trader-cognitive", datura.APPJSON).
						WithDestination("ui").
						WithRole("cognitive").
						WithScope("cognitive").
						WithPayload(marshaled)
					crypto.uiBroadcast.Send(cognitiveArtifact)
				}
			}

			for _, measurement := range measurements {
				crypto.uiBroadcast.Send(
					measurement.WithDestination("ui"),
				)
			}

			if sig, ok := crypto.signals.signals[logic.SourceManifold]; ok {
				if msig, ok := sig.(*manifold.Signal); ok {
					if snapshot, err := msig.FieldSnapshot(time.Now()); err == nil && len(snapshot) > 0 {
						if marshaled, err := sonic.Marshal(snapshot); err == nil {
							artifact := datura.Acquire("manifold-field", datura.APPJSON)
							artifact.WithRole("manifold")
							artifact.WithDestination("ui")
							if artifact.WithPayload(marshaled) != nil {
								crypto.uiBroadcast.Send(artifact)
							}
						}
					}
				}
			}

			crypto.uiBroadcast.Send(
				datura.Acquire(
					"trader", datura.APPJSON,
				).WithDestination(
					"ui",
				).WithRole(
					"tick",
				).WithScope(
					"tick",
				).WithPayload(datura.Map[any]{
					"count":        crypto.tick.Add(1),
					"phase":        crypto.tradingModel,
					"measurements": len(measurements),
					"candidates":   len(actions),
					"chosen":       len(chosen),
					"open":         openHoldingCount(balances, crypto.quoteCurrency),
					"quotes_ready": len(measurements),
					"quotes_total": len(measurements),
					"origins":      originCounts,
					"fluid":        originCounts[string(logic.SourceFluid)],
				}.Marshal()),
			)
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

	crypto.resonance.HydrateMarketFromTree()

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
