package trader

import (
	"context"
	"sort"
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
uiMeasurements returns the latest raw measurement artifact per origin/symbol for
the trader's current-tick view and browser broadcast. Signal.Measure still
persists every produced measurement to the tree as replay evidence; Story,
resonance, the decider, and the UI only need the latest same-tick state per
origin/symbol.
*/
func uiMeasurements(measurements []*datura.Artifact) []*datura.Artifact {
	latest := make(map[string]*datura.Artifact)

	for _, measurement := range measurements {
		if measurement == nil {
			continue
		}

		origin, err := measurement.Origin()
		if err != nil || origin == "" {
			continue
		}

		scope, err := measurement.Scope()
		if err != nil || scope == "" {
			continue
		}

		key := origin + "\x00" + scope
		prior := latest[key]

		if prior == nil || measurement.Timestamp() >= prior.Timestamp() {
			latest[key] = measurement
		}
	}

	keys := make([]string, 0, len(latest))
	for key := range latest {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	out := make([]*datura.Artifact, 0, len(keys))
	for _, key := range keys {
		out = append(out, latest[key])
	}

	return out
}

/*
decisionArtifact builds the backend decision Artifact for one candidate action:
role=decision, scope=symbol, with the verdict (allow/blocked), the decider's
recorded reason, the trader score, and the entry confidence as artifact fields. The verdict and
reason come from the decider's pointer-keyed sets — an admitted candidate is
allowed; a rejected one carries the exact cause (missing source, no slot, below
edge); anything else simply did not clear the edge. No JSON DTO is constructed;
the frontend reads these fields off the decoded artifact directly.
*/
func decisionArtifact(
	action *datura.Artifact,
	chosen map[*datura.Artifact]struct{},
	verdictByAction map[*datura.Artifact]verdict,
	observedAt time.Time,
	seq int64,
) *datura.Artifact {
	symbol, _ := action.Scope()

	return datura.Acquire("trader-decision", datura.APPJSON).
		WithDestination("ui").
		WithRole("decision").
		WithScope(symbol).
		WithPayload(decisionRecord(action, chosen, verdictByAction, observedAt, seq).Marshal())
}

func decisionsArtifact(
	actions []*datura.Artifact,
	chosen map[*datura.Artifact]struct{},
	verdictByAction map[*datura.Artifact]verdict,
	observedAt time.Time,
	seq int64,
) *datura.Artifact {
	decisions := make([]datura.Map[any], 0, len(actions))

	for _, action := range actions {
		decisions = append(decisions, decisionRecord(
			action,
			chosen,
			verdictByAction,
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
	verdictByAction map[*datura.Artifact]verdict,
	observedAt time.Time,
	seq int64,
) datura.Map[any] {
	symbol, _ := action.Scope()
	side, _ := action.Role()

	verdict := "blocked"
	record, hasRecord := verdictByAction[action]
	reason := record.reason
	score := record.score

	if _, isChosen := chosen[action]; isChosen {
		verdict = "allow"
		if reason == "" {
			reason = "admitted"
		}
	} else if !hasRecord || reason == "" {
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
		"score":       score,
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

			// Build the peer snapshot once per tick before measuring, so every
			// signal reads a complete, consistent cross-section.
			crypto.signals.Observe(crypto.crossSection)

			measurements := uiMeasurements(crypto.signals.Measure(crypto.crossSection))

			resonanceMeasurements, resonanceErr := crypto.resonate(measurements)
			if resonanceErr != nil {
				panic(resonanceErr)
			}
			measurements = append(measurements, resonanceMeasurements...)

			// Holdings come from the tree (paper and live both publish balances
			// there via frame.Publish), so the playbook and decider evaluate
			// against the live, fill-mutated ledger.
			balances := holdings(crypto.tree)
			actions, storyErr := crypto.story.Update(measurements, balances)
			if storyErr != nil {
				panic(storyErr)
			}
			originCounts := measurementOriginCounts(measurements)

			// The decision is anchored to when the data was observed (the latest
			// measurement timestamp), not to wall-clock at evaluation time, so
			// replay and live share one clock.
			observedAt := latestObservedTime(measurements)

			// The decider ranks candidates by expected free energy against the
			// manifold field and dispatches only positive-edge entries the ledger
			// does not already hold (plus all protective exits). The single
			// decision point.
			chosen, verdicts := crypto.decider.choose(measurements, actions, balances)
			if deskErr := crypto.desk.Update(chosen); deskErr != nil {
				panic(deskErr)
			}

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

			verdictByAction := make(map[*datura.Artifact]verdict, len(verdicts))
			for _, verdict := range verdicts {
				verdictByAction[verdict.action] = verdict
			}

			// Publish one backend decision Artifact per candidate — role=decision,
			// scope=symbol — carrying the verdict, reason, and entry confidence as
			// artifact fields. No JSON DTO: the frontend renders the raw artifact.
			for _, action := range actions {
				crypto.uiBroadcast.Send(
					decisionArtifact(action, chosenSet, verdictByAction, observedAt, tickCount),
				)
			}
			crypto.uiBroadcast.Send(
				decisionsArtifact(actions, chosenSet, verdictByAction, observedAt, tickCount),
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

				marshaled, marshalErr := sonic.Marshal(map[string]any{"evaluations": evaluations})
				if marshalErr != nil {
					panic(errnie.Err(errnie.Validation, "crypto: marshal playbook walk", marshalErr))
				}

				walkArtifact := datura.Acquire("trader-walk", datura.APPJSON).
					WithDestination("ui").
					WithRole("walk").
					WithScope("walk").
					WithPayload(marshaled)
				crypto.sendUI(walkArtifact)
			}

			// Derive and publish per-symbol cognitive readings (regime sequence,
			// entropy gate, winner class) from this tick's measurement set so the
			// Cortex surface renders real cognitive state, not a simulated beam.
			if cognitive := market.CognitiveReadings(crypto.tree, measurements); len(cognitive) > 0 {
				marshaled, marshalErr := sonic.Marshal(map[string]any{"readings": cognitive})
				if marshalErr != nil {
					panic(errnie.Err(errnie.Validation, "crypto: marshal cognitive readings", marshalErr))
				}

				cognitiveArtifact := datura.Acquire("trader-cognitive", datura.APPJSON).
					WithDestination("ui").
					WithRole("cognitive").
					WithScope("cognitive").
					WithPayload(marshaled)
				crypto.sendUI(cognitiveArtifact)
			}

			// Derive per-position economics (quantity, entry, mark, unrealized
			// P&L) on the backend from the ledger, fills, and marks in the tree,
			// and publish them as a positions Artifact. The terminal renders this
			// raw — it does not recompute P&L from scattered readings.
			positions, positionsErr := market.PositionReadings(crypto.tree, crypto.quoteCurrency)
			if positionsErr != nil {
				panic(positionsErr)
			}
			if len(positions) > 0 {
				marshaled, marshalErr := sonic.Marshal(map[string]any{
					"positions":   positions,
					"quote":       crypto.quoteCurrency,
					"observed_at": observedAt.UnixMilli(),
				})
				if marshalErr != nil {
					panic(errnie.Err(errnie.Validation, "crypto: marshal position readings", marshalErr))
				}

				positionsArtifact := datura.Acquire("trader-positions", datura.APPJSON).
					WithDestination("ui").
					WithRole("positions").
					WithScope("positions").
					WithPayload(marshaled)
				crypto.sendUI(positionsArtifact)
			}

			for _, measurement := range uiMeasurements(measurements) {
				crypto.sendUI(measurement.WithDestination("ui"))
			}

			if sig, ok := crypto.signals.signals[logic.SourceManifold]; ok {
				if msig, ok := sig.(*manifold.Signal); ok {
					snapshot, snapshotErr := msig.FieldSnapshot(observedAt)
					if snapshotErr != nil {
						panic(errnie.Err(errnie.Validation, "crypto: manifold field snapshot", snapshotErr))
					}
					if len(snapshot) > 0 {
						marshaled, marshalErr := sonic.Marshal(snapshot)
						if marshalErr != nil {
							panic(errnie.Err(errnie.Validation, "crypto: marshal manifold field", marshalErr))
						}

						artifact := datura.Acquire("manifold-field", datura.APPJSON)
						artifact.WithRole("manifold")
						artifact.WithDestination("ui")
						if artifact.WithPayload(marshaled) == nil {
							panic(errnie.Err(errnie.Validation, "crypto: manifold field payload rejected", nil))
						}
						crypto.sendUI(artifact)
					}
				}
			}

		}
	}
}

func (crypto *Crypto) publishTick(artifact *datura.Artifact) {
	if artifact == nil {
		panic(errnie.Err(errnie.Validation, "crypto: nil tick artifact", nil))
	}

	if crypto.tree != nil {
		crypto.tree.InsertArtifact(artifact.Prefix("role", "timestamp"), artifact)
	}

	crypto.sendUI(artifact)
}

func (crypto *Crypto) sendUI(artifact *datura.Artifact) {
	if artifact == nil {
		panic(errnie.Err(errnie.Validation, "crypto: nil ui artifact", nil))
	}

	if err := crypto.uiBroadcast.Send(artifact); err != nil {
		panic(errnie.Err(errnie.Validation, "crypto: ui broadcast failed", err))
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
) ([]*datura.Artifact, error) {
	if len(measurements) == 0 {
		return nil, nil
	}
	if crypto.resonance == nil {
		return nil, errnie.Err(errnie.Validation, "crypto: resonance signal is nil", nil)
	}

	crypto.resonance.HydrateMarketFromTree()

	seen := make(map[string]struct{})
	scopes := make([]string, 0, len(measurements))

	for _, measurement := range measurements {
		scope, scopeErr := measurement.Scope()
		if scopeErr != nil {
			return nil, errnie.Err(
				errnie.Validation,
				"crypto: failed to get measurement scope",
				scopeErr,
			)
		}

		if scope == "" {
			return nil, errnie.Err(
				errnie.Validation,
				"crypto: measurement has empty scope",
				nil,
			)
		}

		if _, ok := seen[scope]; ok {
			continue
		}

		seen[scope] = struct{}{}
		scopes = append(scopes, scope)
	}

	if len(scopes) == 0 {
		return nil, errnie.Err(
			errnie.Validation,
			"crypto: measurements produced no resonance scopes",
			nil,
		)
	}

	settled, settleErr := crypto.resonance.SettleScopes(scopes)
	if settleErr != nil {
		return nil, errnie.Err(
			errnie.Validation,
			"crypto: failed to settle resonance scopes",
			settleErr,
		)
	}

	resonances := make([]*datura.Artifact, 0, len(settled))

	for scope, measurement := range settled {
		if measurement == nil {
			return nil, errnie.Err(
				errnie.Validation,
				"crypto: nil resonance measurement for "+scope,
				nil,
			)
		}

		resonances = append(resonances, measurement)
	}

	return resonances, nil
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
