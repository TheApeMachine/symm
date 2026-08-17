package broker

import (
	"math"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/types"
)

/*
passageTracker accumulates one open lot's first-passage evidence while it
lives. Every executable mark contributes one feature row and moves the lot's
realized adverse and favorable excursions, both stated in the risk distances
the lot was entered under so one symbol's history can inform another's.
*/
type passageTracker struct {
	episode types.PassageEpisode
	regime  string
	count   int
}

func newPassageTracker(
	position *Position,
	entryPrice *decimal.Decimal,
	horizon int,
) *passageTracker {
	regime := ""

	if position != nil {
		regime = position.Decision.OpportunityType

		if regime == "" {
			regime = position.Decision.Cause
		}
	}

	tracker := &passageTracker{
		regime: regime,
		episode: types.PassageEpisode{
			PositionID:   position.Decision.ID,
			Symbol:       position.Decision.Symbol,
			OpenedTick:   time.Now().UTC().UnixNano(),
			Horizon:      float64(horizon),
			Entry:        entryPrice.Float64(),
			HardFloor:    hardFloorOf(position, entryPrice),
			ProfitLine:   profitLineOf(position),
			ArmLine:      armLineOf(position),
			Observations: []types.PassageFeatures{{Regime: regime}},
		},
	}

	return tracker
}

/*
observe folds one executable mark into the running excursion record.
*/
func (tracker *passageTracker) observe(
	position *Position,
	mark *decimal.Decimal,
) {
	if tracker == nil || position == nil || mark == nil || mark.Sign() <= 0 {
		return
	}

	plan := lotPlan(position)

	if plan == nil || plan.RiskDistance == nil || plan.RiskDistance.Sign() <= 0 {
		return
	}

	if tracker.episode.Entry <= 0 {
		return
	}

	drawdown := (mark.Float64() - tracker.episode.Entry) /
		plan.RiskDistance.Float64()

	if math.IsNaN(drawdown) || math.IsInf(drawdown, 0) {
		return
	}

	tracker.count++

	if -drawdown > tracker.episode.MaxAdverse {
		tracker.episode.MaxAdverse = -drawdown
	}

	if drawdown > tracker.episode.MaxFavorable {
		tracker.episode.MaxFavorable = drawdown
	}

	liquidity := 1.0

	if tick := liveTick(position); tick != nil &&
		tick.Bid != nil && tick.Ask != nil &&
		tick.Bid.Sign() > 0 && tick.Ask.Sign() > 0 &&
		plan.EntryNoiseBand != nil && plan.EntryNoiseBand.Sign() > 0 {
		spread := decimal.NewFromInt64(0).Add(tick.Ask).Sub(tick.Bid)
		liquidity = spread.Float64() / plan.EntryNoiseBand.Float64()
	}

	age := 1.0

	if tracker.episode.Horizon > 0 {
		age = float64(tracker.count) / tracker.episode.Horizon
	}

	tracker.episode.Observations = append(
		tracker.episode.Observations,
		types.PassageFeatures{
			Drawdown:  drawdown,
			Age:       age,
			Liquidity: liquidity,
			Regime:    tracker.regime,
		},
	)
}

/*
complete classifies the lot's outcome from the regulator's own trigger and
returns the finished episode. A false result means the exit was censored — it
was decided by neither boundary — so no outcome is claimed for it.
*/
func (tracker *passageTracker) complete(
	position *Position,
) (types.PassageEpisode, bool) {
	if tracker == nil {
		return types.PassageEpisode{}, false
	}

	tracker.episode.ClosedTick = time.Now().UTC().UnixNano()
	tracker.episode.ExitReason = lotTriggerReason(position)

	outcome, decided := passageOutcome(tracker.episode.ExitReason)

	if !decided {
		tracker.episode.Censored = true
		return tracker.episode, false
	}

	tracker.episode.Outcome = outcome

	return tracker.episode, true
}

/*
passageOutcome maps a stoploss trigger onto the competing first-passage
outcomes. Triggers that either boundary owns decide the episode; everything
else censors it.
*/
func passageOutcome(trigger string) (types.PassageOutcome, bool) {
	switch trigger {
	case types.TriggerHardFloor:
		return types.OutcomeLossFirst, true
	case types.TriggerProtectedFloor,
		types.TriggerTrailingFloor,
		types.TriggerProfitStagnation,
		types.TriggerPumpMomentumLost:
		return types.OutcomeProfitFirst, true
	case types.TriggerHorizonExpired:
		return types.OutcomeTimeout, true
	default:
		return "", false
	}
}

func lotPlan(position *Position) *types.RiskPlan {
	if position == nil || position.Holding == nil ||
		position.Holding.Stoploss == nil {
		return nil
	}

	return position.Holding.Stoploss.Plan
}

func liveTick(position *Position) *kraken.TickerData {
	if position == nil || position.price == nil || position.Holding == nil {
		return nil
	}

	return position.price.Tick(position.Holding.Symbol)
}

func lotTriggerReason(position *Position) string {
	if position == nil || position.Holding == nil ||
		position.Holding.Stoploss == nil {
		return ""
	}

	return position.Holding.Stoploss.TriggerReason
}

func hardFloorOf(position *Position, entryPrice *decimal.Decimal) float64 {
	plan := lotPlan(position)

	if plan == nil || plan.RiskDistance == nil || entryPrice == nil {
		return 0
	}

	return entryPrice.Float64() - plan.RiskDistance.Float64()
}

func profitLineOf(position *Position) float64 {
	if position == nil || position.Holding == nil ||
		position.Holding.Stoploss == nil ||
		position.Holding.Stoploss.ProfitLine == nil {
		return 0
	}

	return position.Holding.Stoploss.ProfitLine.Float64()
}

func armLineOf(position *Position) float64 {
	if position == nil || position.Holding == nil ||
		position.Holding.Stoploss == nil ||
		position.Holding.Stoploss.ArmAt == nil {
		return 0
	}

	return position.Holding.Stoploss.ArmAt.Float64()
}

/*
foldPassage completes a removed lot's episode and folds it into the desk's
first-passage model. Lots that never filled, and exits neither boundary
decided, contribute no outcome.
*/
func (desk *Desk) foldPassage(position *Position) {
	if desk == nil || desk.passage == nil ||
		position == nil || position.passage == nil {
		return
	}

	episode, decided := position.passage.complete(position)

	if !decided {
		return
	}

	desk.passage.Fold(episode)
}

/*
PassageAdverseQuantile exposes the winners' calibrated adverse-excursion
quantile for stop geometry, in the risk-distance multiples the finishing lots
carried.
*/
func (desk *Desk) PassageAdverseQuantile(confidence float64) (float64, bool) {
	if desk == nil || desk.passage == nil {
		return 0, false
	}

	return desk.passage.AdverseQuantile(confidence)
}
