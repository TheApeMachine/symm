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

/*
latestObservedTime returns the most recent measurement timestamp in the batch as
the decision's wall-clock anchor. Decisions are stamped by when the data was
observed, not by clock-at-evaluation, so replay and live trade off one timeline.
*/
func latestObservedTime(measurements []*datura.Artifact) time.Time {
	latest := int64(0)

	for _, measurement := range measurements {
		if stamp := measurement.Timestamp(); stamp > latest {
			latest = stamp
		}
	}

	if latest == 0 {
		return time.Now().UTC()
	}

	return time.Unix(0, latest).UTC()
}

/*
decisionArtifact builds the backend decision Artifact for one candidate action:
role=decision, scope=symbol, with the verdict (allow/blocked), the decider's
recorded reason, and the entry confidence as artifact fields. The verdict and
reason come from the decider's pointer-keyed sets — an admitted candidate is
allowed; a rejected one carries the exact cause (missing source, no slot, below
edge); anything else simply did not clear the edge. No JSON DTO is constructed;
the frontend reads these fields off the decoded artifact directly.
*/
func decisionArtifact(
	action *datura.Artifact,
	chosen map[*datura.Artifact]struct{},
	rejectionByAction map[*datura.Artifact]string,
	observedAt time.Time,
	seq int64,
) *datura.Artifact {
	symbol, _ := action.Scope()

	return datura.Acquire("trader-decision", datura.APPJSON).
		WithDestination("ui").
		WithRole("decision").
		WithScope(symbol).
		WithPayload(decisionRecord(action, chosen, rejectionByAction, observedAt, seq).Marshal())
}

func decisionsArtifact(
	actions []*datura.Artifact,
	chosen map[*datura.Artifact]struct{},
	rejectionByAction map[*datura.Artifact]string,
	observedAt time.Time,
	seq int64,
) *datura.Artifact {
	decisions := make([]datura.Map[any], 0, len(actions))

	for _, action := range actions {
		decisions = append(decisions, decisionRecord(
			action,
			chosen,
			rejectionByAction,
			observedAt,
			seq,
		))
	}

	return datura.Acquire("trader-decisions", datura.APPJSON).
		WithDestination("ui").
		WithRole("decisions").
		WithScope("trader").
		WithPayload(datura.Map[any]{
			"decisions":   decisions,
			"observed_at": observedAt.UnixMilli(),
			"seq":         seq,
		}.Marshal())
}

func decisionRecord(
	action *datura.Artifact,
	chosen map[*datura.Artifact]struct{},
	rejectionByAction map[*datura.Artifact]string,
	observedAt time.Time,
	seq int64,
) datura.Map[any] {
	symbol, _ := action.Scope()
	side, _ := action.Role()

	verdict := "blocked"
	reason, hasReason := rejectionByAction[action]

	if _, isChosen := chosen[action]; isChosen {
		verdict = "allow"
		reason = "admitted"
	} else if !hasReason {
		reason = "below edge"
	}

	return datura.Map[any]{
		"action_id":   actionID(action),
		"symbol":      symbol,
		"side":        side,
		"type":        datura.Peek[string](action, "type"),
		"price":       datura.Peek[float64](action, "price"),
		"quantity":    datura.Peek[float64](action, "quantity"),
		"confidence":  datura.Peek[float64](action, "entry_confidence"),
		"verdict":     verdict,
		"why":         reason,
		"observed_at": observedAt.UnixMilli(),
		"seq":         seq,
	}
}

func tickArtifact(
	seq int64,
	tradingModel string,
	measurements int,
	candidates int,
	chosen int,
	open int,
	originCounts map[string]int,
) *datura.Artifact {
	if originCounts == nil {
		originCounts = map[string]int{}
	}

	return datura.Acquire(
		"trader", datura.APPJSON,
	).WithDestination(
		"ui",
	).WithRole(
		"tick",
	).WithScope(
		"tick",
	).WithPayload(datura.Map[any]{
		"count":        seq,
		"phase":        tradingModel,
		"measurements": measurements,
		"candidates":   candidates,
		"chosen":       chosen,
		"open":         open,
		"quotes_ready": measurements,
		"quotes_total": measurements,
		"origins":      originCounts,
		"fluid":        originCounts[string(logic.SourceFluid)],
	}.Marshal())
}

func actionID(action *datura.Artifact) string {
	if action == nil {
		return ""
	}

	uuid, err := action.Uuid()
	if err != nil || len(uuid) == 0 {
		return ""
	}

	return string(uuid)
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
	// The loop is paced by the data, not a fixed wall-clock interval: after each
	// pass it sleeps for the cadence the signals actually observed (bounded).
	// Empty passes still emit a tick heartbeat so the frontend can distinguish
	// a quiet market from a dead trader loop.
	timer := time.NewTimer(crypto.signals.PollInterval())
	defer timer.Stop()

	for {
		select {
		case <-crypto.ctx.Done():
			return nil
		case <-timer.C:
			timer.Reset(crypto.signals.PollInterval())
			tickCount := crypto.tick.Add(1)

			crypto.publishTick(tickArtifact(
				tickCount,
				crypto.tradingModel,
				0,
				0,
				0,
				0,
				nil,
			))

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

			// The decision is anchored to when the data was observed (the latest
			// measurement timestamp), not to wall-clock at evaluation time, so
			// replay and live share one clock.
			observedAt := latestObservedTime(measurements)

			// The decider ranks candidates by expected free energy against the
			// manifold field and dispatches only positive-edge entries the ledger
			// does not already hold (plus all protective exits). The single
			// decision point.
			chosen, rejections := crypto.decider.choose(measurements, actions, balances)
			errnie.Error(crypto.desk.Update(chosen))

			crypto.publishTick(tickArtifact(
				tickCount,
				crypto.tradingModel,
				len(measurements),
				len(actions),
				len(chosen),
				openHoldingCount(balances, crypto.quoteCurrency),
				originCounts,
			))

			// Identity is the artifact pointer, not a symbol/side/type tuple: the
			// decider passes the very same candidate artifacts through to chosen,
			// so a chosen entry is the one whose pointer is in the set. Two
			// candidates for the same symbol can never be mislabeled by a shared
			// tuple.
			chosenSet := make(map[*datura.Artifact]struct{}, len(chosen))
			for _, choice := range chosen {
				chosenSet[choice] = struct{}{}
			}

			rejectionByAction := make(map[*datura.Artifact]string, len(rejections))
			for _, rejection := range rejections {
				rejectionByAction[rejection.action] = rejection.reason
			}

			// Publish one backend decision Artifact per candidate — role=decision,
			// scope=symbol — carrying the verdict, reason, and entry confidence as
			// artifact fields. No JSON DTO: the frontend renders the raw artifact.
			for _, action := range actions {
				crypto.uiBroadcast.Send(
					decisionArtifact(action, chosenSet, rejectionByAction, observedAt, tickCount),
				)
			}
			crypto.uiBroadcast.Send(
				decisionsArtifact(actions, chosenSet, rejectionByAction, observedAt, tickCount),
			)

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

			// Derive per-position economics (quantity, entry, mark, unrealized
			// P&L) on the backend from the ledger, fills, and marks in the tree,
			// and publish them as a positions Artifact. The terminal renders this
			// raw — it does not recompute P&L from scattered readings.
			if positions := market.PositionReadings(crypto.tree, crypto.quoteCurrency); len(positions) > 0 {
				if marshaled, err := sonic.Marshal(map[string]any{
					"positions":   positions,
					"quote":       crypto.quoteCurrency,
					"observed_at": observedAt.UnixMilli(),
				}); err == nil {
					positionsArtifact := datura.Acquire("trader-positions", datura.APPJSON).
						WithDestination("ui").
						WithRole("positions").
						WithScope("positions").
						WithPayload(marshaled)
					crypto.uiBroadcast.Send(positionsArtifact)
				}
			}

			for _, measurement := range measurements {
				crypto.uiBroadcast.Send(
					measurement.WithDestination("ui"),
				)
			}

			if sig, ok := crypto.signals.signals[logic.SourceManifold]; ok {
				if msig, ok := sig.(*manifold.Signal); ok {
					if snapshot, err := msig.FieldSnapshot(observedAt); err == nil && len(snapshot) > 0 {
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

		}
	}
}

func (crypto *Crypto) publishTick(artifact *datura.Artifact) {
	if artifact == nil {
		return
	}

	if crypto.tree != nil {
		crypto.tree.InsertArtifact(artifact.Prefix("role", "timestamp"), artifact)
	}

	crypto.uiBroadcast.Send(artifact)
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
