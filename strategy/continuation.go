package strategy

import (
	"math"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/audit"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/types"
)

/*
passageEpisode is one open lot's life as the first-passage model will eventually
read it: the states it passed through, and — once it finishes — which boundary
it reached.

The states are held rather than scored as they happen because the label does not
exist yet. Whether a drawdown at minute two was survivable is only knowable at
minute forty, so every state waits for its episode to end before it teaches the
model anything.
*/
type passageEpisode struct {
	symbol       string
	openedTick   int64
	horizon      float64
	entryNoise   *decimal.Decimal
	observations []types.PassageFeatures
	finished     bool
	armed        bool
	lastAge      float64
	lastTick     int64
	lastTrigger  string
	entry        float64
	hardFloor    float64
	profitLine   float64
	armLine      float64
	maxAdverse   float64
	maxFavorable float64
}

/*
observe records one state, and notes if the lot has ever cleared its profit
line. Arming is sticky because reaching the profit line is the outcome being
predicted: a lot that armed and later gave the profit back still answered the
question with "profit first".
*/
func (episode *passageEpisode) observe(
	features types.PassageFeatures,
	snapshot types.StopSnapshot,
	tick int64,
) {
	episode.observations = append(episode.observations, features)
	episode.lastAge = features.Age
	episode.lastTick = tick
	episode.armed = episode.armed || snapshot.ProfitArmed

	if snapshot.MaxAdverse != nil {
		episode.maxAdverse = math.Min(
			episode.maxAdverse,
			snapshot.MaxAdverse.Float64(),
		)
	}

	if snapshot.MaxFavorable != nil {
		episode.maxFavorable = math.Max(
			episode.maxFavorable,
			snapshot.MaxFavorable.Float64(),
		)
	}

	if snapshot.Entry != nil {
		episode.entry = snapshot.Entry.Float64()
	}

	if snapshot.HardFloor != nil {
		episode.hardFloor = snapshot.HardFloor.Float64()
	}

	if snapshot.ProfitLine != nil {
		episode.profitLine = snapshot.ProfitLine.Float64()
	}

	if snapshot.ArmLine != nil {
		episode.armLine = snapshot.ArmLine.Float64()
	}

	if snapshot.TriggerReason != "" {
		episode.lastTrigger = snapshot.TriggerReason
	}
}

/*
record renders the finished episode for the audit trail.
*/
func (episode *passageEpisode) record(
	id string,
	outcome types.PassageOutcome,
	labelled bool,
) types.PassageEpisode {
	return types.PassageEpisode{
		PositionID:   id,
		Symbol:       episode.symbol,
		OpenedTick:   episode.openedTick,
		ClosedTick:   episode.lastTick,
		Horizon:      episode.horizon,
		Outcome:      outcome,
		Censored:     !labelled,
		ExitReason:   episode.lastTrigger,
		HardFloor:    episode.hardFloor,
		ProfitLine:   episode.profitLine,
		ArmLine:      episode.armLine,
		Entry:        episode.entry,
		MaxAdverse:   episode.maxAdverse,
		MaxFavorable: episode.maxFavorable,
		Observations: episode.observations,
	}
}

/*
outcome states which boundary the finished lot reached first, or reports that it
reached neither for a reason that teaches nothing.

The censored case is the important one. A lot closed by arbitration, by a
rotation, or by the operator never had its patience tested, and folding it in as
a timeout would tell the model that waiting is safe using exactly the episodes
where waiting did not happen.
*/
func (episode *passageEpisode) outcome() (types.PassageOutcome, bool) {
	switch {
	case episode.armed:
		return types.OutcomeProfitFirst, true
	case episode.lastTrigger == types.TriggerProtectedGiveback,
		episode.lastTrigger == types.TriggerProfitFailSafe:
		return types.OutcomeProfitFirst, true
	case episode.lastTrigger == types.TriggerHardRisk:
		return types.OutcomeLossFirst, true
	case episode.lastAge >= 1:
		return types.OutcomeTimeout, true
	default:
		return "", false
	}
}

/*
regimeOf reduces a symbol's structural categories to the one word the model
buckets on.

Reversal wins over momentum when both are present, because a lot held into a
reversal is the case the model most needs to separate and the optimistic reading
is the expensive mistake.
*/
func regimeOf(thesis *types.Thesis, symbol string) string {
	if thesis == nil || len(thesis.Categories) == 0 {
		return "unclassified"
	}

	regime := "neutral"

	for _, category := range thesis.Categories[symbol] {
		switch category.Type {
		case types.CategoryActiveReversal,
			types.CategoryMechanicalCollapse,
			types.CategoryToxicBluff,
			types.CategoryExhaustion,
			types.CategoryFadedExhaustion,
			types.CategoryThermalExhaustion:
			return "reversal"
		case types.CategoryVerticalIgnition,
			types.CategoryFrenzy,
			types.CategoryAggressiveDrive,
			types.CategoryLiquidityShock,
			types.CategoryLoadedImbalance:
			regime = "momentum"
		}
	}

	return regime
}

/*
passageFeatures states one open lot's position relative to its own boundaries.

Absence anywhere returns false rather than a zero: a drawdown measured against a
risk distance that was never derived is not a small drawdown, it is no reading
at all.
*/
func passageFeatures(
	snapshot types.StopSnapshot,
	forecast candidate,
	episode *passageEpisode,
	regime string,
	tick int64,
) (types.PassageFeatures, bool) {
	if !snapshot.Present || snapshot.Mark == nil || snapshot.Entry == nil ||
		snapshot.RiskDistance == nil || snapshot.RiskDistance.Sign() <= 0 {
		return types.PassageFeatures{}, false
	}

	drawdown := snapshot.Mark.SetScale(12).
		Sub(snapshot.Entry).
		Div(snapshot.RiskDistance).
		Float64()

	age := 0.0

	if episode.horizon > 0 {
		age = float64(tick-episode.openedTick) / episode.horizon
	}

	liquidity := 1.0

	if episode.entryNoise != nil && episode.entryNoise.Sign() > 0 &&
		snapshot.NoiseBand != nil {
		liquidity = snapshot.NoiseBand.SetScale(12).Div(episode.entryNoise).Float64()
	}

	features := types.PassageFeatures{
		Drawdown:  drawdown,
		Age:       age,
		Forecast:  forecast.ExecutableFraction(),
		Liquidity: liquidity,
		Regime:    regime,
	}

	if math.IsNaN(features.Drawdown) || math.IsInf(features.Drawdown, 0) {
		return types.PassageFeatures{}, false
	}

	return features, true
}

/*
passageRoom prices how far the lot can still travel in each direction, as a
fraction of the price it would liquidate at.

Both are floored at zero. A mark already through a boundary has no room left on
that side, and a negative distance would flip the sign of the term it feeds.
*/
func passageRoom(snapshot types.StopSnapshot) (upside, downside float64, ok bool) {
	if snapshot.Mark == nil || snapshot.Mark.Sign() <= 0 ||
		snapshot.ArmLine == nil || snapshot.HardFloor == nil {
		return 0, 0, false
	}

	mark := snapshot.Mark.SetScale(12)

	upside = math.Max(0, snapshot.ArmLine.SetScale(12).Sub(mark).Div(mark).Float64())
	downside = math.Max(0, mark.Sub(snapshot.HardFloor).Div(mark).Float64())

	return upside, downside, true
}

/*
scorePassage records, for one open lot, the historical value of reaching its
protected profit before its hard floor.

It runs only before profit protection arms. After that the question has already
been answered — the lot reached its profit line — and the regulator's own
protected floor governs what happens next. Asking a probability model to
second-guess a floor that is defending realised profit would be replacing a
guarantee with an estimate.

The verdict is diagnostic only. It never widens the hard floor, suppresses a
stop, or emits an exit; an unready model, missing geometry or an unpriced state
returns false and leaves the decision without passage fields.
*/
func (evaluator Evaluator) scorePassage(
	thesis *types.Thesis,
	position *broker.Position,
	forecast candidate,
	exitCostFraction float64,
) (types.PassageScenario, float64, bool) {
	snapshot := position.StopSnapshot()

	if !snapshot.Present {
		return types.PassageScenario{}, 0, false
	}

	episode := evaluator.episode(position, forecast, snapshot, thesis.Tick)

	if episode.finished {
		return types.PassageScenario{}, 0, false
	}

	features, ok := passageFeatures(
		snapshot, forecast, episode, regimeOf(thesis, position.Holding.Symbol),
		thesis.Tick,
	)

	if !ok {
		return types.PassageScenario{}, 0, false
	}

	episode.observe(features, snapshot, thesis.Tick)

	if snapshot.ProfitArmed || features.Age >= 1 {
		evaluator.finishEpisode(position.ID, episode, snapshot.TriggerReason)
		return types.PassageScenario{}, 0, false
	}

	upside, downside, priced := passageRoom(snapshot)

	if !priced {
		return types.PassageScenario{}, 0, false
	}

	scenario := evaluator.passage.Scenario(features)
	holdEV := scenario.HoldEV(upside, downside, exitCostFraction)

	return scenario, holdEV, scenario.Ready
}

/*
episode returns this lot's in-flight episode, opening one on first sight.

The horizon and the execution-noise band are captured once, at the state the lot
was first scored in, because both are what the later features are stated
relative to. Re-reading them every tick would make a lot look younger as its
forecast was renewed and its liquidity unchanged as the book widened.
*/
func (evaluator Evaluator) episode(
	position *broker.Position,
	forecast candidate,
	snapshot types.StopSnapshot,
	tick int64,
) *passageEpisode {
	if existing, found := evaluator.episodes[position.ID]; found {
		return existing
	}

	horizon := float64(forecast.Epoch) - float64(tick)

	if horizon <= 0 {
		horizon = 1
	}

	episode := &passageEpisode{
		symbol:     position.Holding.Symbol,
		openedTick: tick,
		horizon:    horizon,
		entryNoise: snapshot.NoiseBand,
	}

	evaluator.episodes[position.ID] = episode

	return episode
}

/*
retire folds one finished lot's states into the model and forgets it.

Every state the lot passed through is folded in under the same label, because
the episode ended at the first boundary it reached: whatever ended it is the
answer to "from here, which comes first" at every point along the way.

An unlabelled episode is dropped rather than counted. Forgetting the lot is
unconditional either way, so a censored episode cannot be retired twice or leak.
*/
func (evaluator Evaluator) finishEpisode(
	id string,
	episode *passageEpisode,
	trigger string,
) {
	if episode == nil || episode.finished {
		return
	}

	if trigger != "" {
		episode.lastTrigger = trigger
	}

	episode.finished = true

	outcome, labelled := episode.outcome()

	/*
		The record is written whether or not the episode could be labelled, and
		the censored ones are written precisely because they are censored. An
		offline fit needs to know which lots were closed before their patience
		was tested; dropping them here would leave the corpus looking like every
		trade ran to a boundary, which is the same survivorship bias that makes
		the in-process model refuse to count them.
	*/
	errnie.Error(audit.Record(
		evaluator.recorder, "passage", episode.record(id, outcome, labelled),
	))

	if !labelled || evaluator.passage == nil {
		return
	}

	evaluator.passage.ObserveEpisode(episode.observations, outcome)
}

func (evaluator Evaluator) retire(id, trigger string) {
	episode, tracked := evaluator.episodes[id]

	if !tracked {
		return
	}

	evaluator.finishEpisode(id, episode, trigger)
	delete(evaluator.episodes, id)
}

/*
retireEpisodes folds every finished lot's states into the model and forgets it.

This is the only place the model learns. It runs after the pass over open
positions so a lot that closed during this tick is retired with the last state
it was actually seen in.
*/
func (evaluator Evaluator) retireEpisodes(desk *broker.Desk) {
	if desk == nil || evaluator.passage == nil {
		return
	}

	known := make(map[string]struct{}, len(evaluator.episodes))

	for position := range desk.Positions() {
		known[position.ID] = struct{}{}

		if position.Status == types.OPEN {
			continue
		}

		// The regulator may have fired on the same tick the lot closed, so the
		// trigger is re-read here rather than trusted from the last scoring
		// pass, which ran before the exit.
		evaluator.retire(position.ID, position.StopSnapshot().TriggerReason)
	}

	// A lot the desk has forgotten entirely cannot be labelled, so its states
	// are dropped rather than guessed at.
	for id := range evaluator.episodes {
		if _, found := known[id]; !found {
			delete(evaluator.episodes, id)
		}
	}
}
