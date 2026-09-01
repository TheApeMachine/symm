package strategy

import (
	"context"
	"fmt"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/spf13/viper"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/audit"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/system"
	"github.com/theapemachine/symm/types"
)

/*
Planner is the sole live entry authority. It observes every enriched market
envelope, remains dormant until its binary classifications demonstrate
positive prequential skill, and only submits entries. Desk-owned Stoploss
instances retain exclusive exit authority.
*/
type Planner struct {
	cancel                context.CancelFunc
	desk                  *broker.Desk
	predictor             *directionalPredictor
	allocation            *Allocation
	recorder              *audit.Recorder
	minimumProbability    float64
	maxAllocationFraction float64
}

func NewPlanner(
	ctx context.Context,
	recorder *audit.Recorder,
	desk *broker.Desk,
) (*Planner, error) {
	if desk == nil {
		return nil, fmt.Errorf("planner: desk required")
	}

	config, err := system.Cfg.PlannerPolicy()

	if err != nil {
		return nil, err
	}

	predictor, err := newDirectionalPredictor(directionalConfig{
		initialVariance:       viper.GetFloat64("market.forecast.rls.initial_variance"),
		forgettingFactor:      viper.GetFloat64("market.forecast.rls.forgetting_factor"),
		calibrationConfidence: viper.GetFloat64("market.forecast.rls.calibration_confidence"),
	})

	if err != nil {
		return nil, err
	}

	if config.MinimumEntryProbability <= system.UninformativeDirectionConfidence ||
		config.MinimumEntryProbability >= 1 {
		return nil, fmt.Errorf("planner: admission probability must be in (0.5, 1)")
	}

	if config.MaxAllocationFraction <= 0 || config.MaxAllocationFraction > 1 {
		return nil, fmt.Errorf("planner: allocation fraction must be in (0, 1]")
	}

	ctx, cancel := context.WithCancel(ctx)

	return &Planner{
		cancel:                cancel,
		desk:                  desk,
		predictor:             predictor,
		allocation:            NewAllocation(ctx, desk),
		recorder:              recorder,
		minimumProbability:    config.MinimumEntryProbability,
		maxAllocationFraction: config.MaxAllocationFraction,
	}, nil
}

/* Step consumes one envelope once and always emits a decision when calibrated. */
func (planner *Planner) Step(envelope *types.Envelope) *types.Envelope {
	if envelope == nil {
		return envelope
	}

	if err := planner.predictor.observe(envelope); err != nil {
		errnie.Error(errnie.Err(errnie.UnprocessableContent, "planner: observe envelope", err))
		return envelope
	}

	if envelope.TypeID != types.EnvelopeTicker {
		return envelope
	}

	forecast, err := planner.forecast(envelope)

	if err != nil {
		errnie.Error(errnie.Err(errnie.UnprocessableContent, "planner: forecast ticker", err))
		return envelope
	}

	if planner.desk.Holding(forecast.symbol) > 0 {
		return envelope
	}

	// The planner is live on the very first ticker frame: it emits a round and
	// a decision even before the RLS heads calibrate, so downstream modules see
	// an output immediately and weigh its maturity themselves. The decision is
	// structurally complete and truthful (PredictiveReady=false while heads are
	// uncalibrated); it simply never carries an enter action until both heads
	// clear their own admission thresholds.
	decision, round := planner.preDecision(forecast)

	if forecast.directionReady && forecast.profitabilityReady {
		decision = planner.decide(forecast)
		round.Decisions = []*types.Decision{decision}

		if decision.Action == types.ActionEnter {
			round.Outcome = "entry"
		} else {
			round.Outcome = "admission"
		}
	}

	envelope.StrategyRound = round

	if decision.Action == types.ActionEnter {
		planner.execute(decision, round)
	}

	if err := audit.RecordAs(planner.recorder, audit.DecisionBatchEvent, round); err != nil {
		errnie.Error(errnie.Err(errnie.IO, "planner: record decision", err))
	}

	return envelope
}

/*
preDecision builds the calibrated-but-not-yet-admitted decision for one ticker
frame. It mirrors decide's full shape so the strategy surface always receives a
well-formed round, but it keeps Action=hold and reports the exact reason the
heads are not ready, so downstream maturity logic can discount it correctly. The
engine surface's phase slot receives the concise admission word, never the
verbose reason, which stays on the decision for the detail surface.
*/
func (planner *Planner) preDecision(forecast *directionalForecast) (*types.Decision, *types.StrategyRound) {
	reason := "planner: calibrated probabilities do not clear admission"

	decision := types.NewDecision(types.ActionNothing, forecast.symbol)
	decision.At = forecast.at
	decision.Direction = 1
	decision.PredictiveReady = false

	switch {
	case !forecast.directionReady && !forecast.profitabilityReady:
		decision.PredictiveStatus = "uncalibrated-direction-and-profitability"
	case !forecast.directionReady:
		decision.PredictiveStatus = "uncalibrated-direction"
	default:
		decision.PredictiveStatus = "uncalibrated-profitability"
	}

	decision.TaskSkill = min(
		forecast.directionSkillLowerBound,
		forecast.profitSkillLowerBound,
	)
	decision.TaskSkillReady = false
	decision.ForecastSource = "full-observation-direction"
	decision.ForecastModel = "streaming-feature-association-rls-v1"
	decision.Forecast = &forecast.directionOutput
	decision.ForecastHorizon = 1
	decision.CalibrationCount = min(
		forecast.directionCalibration,
		forecast.profitCalibration,
	)
	decision.AllocationClass = "none"
	decision.Reason = reason
	decision.Cause = "precursor observations"
	decision.Alternatives = map[string]float64{
		"probability:up":                  forecast.probabilityUp,
		"probability:profitable":          forecast.probabilityProfitable,
		"skill:direction_lower_bound":     forecast.directionSkillLowerBound,
		"skill:profitability_lower_bound": forecast.profitSkillLowerBound,
		"features:direction":              float64(forecast.directionFeatures),
		"features:profitability":          float64(forecast.profitFeatures),
	}

	return decision, &types.StrategyRound{
		Symbol:    forecast.symbol,
		Evaluated: true,
		Outcome:   "admission",
		Decisions: []*types.Decision{decision},
	}
}

func (planner *Planner) forecast(envelope *types.Envelope) (*directionalForecast, error) {
	ticker := envelope.TickerData

	if ticker.Symbol == "" || ticker.Bid == nil || ticker.Ask == nil ||
		ticker.Bid.Sign() <= 0 || ticker.Ask.Sign() <= 0 {
		return nil, fmt.Errorf("planner: ticker symbol and positive bid/ask required")
	}

	if ticker.Timestamp.IsZero() {
		return nil, fmt.Errorf("planner: ticker event time required")
	}

	reference := decimal.NewFromInt64(0).Add(ticker.Bid).Add(ticker.Ask).Div(
		decimal.NewFromInt64(2),
	).Float64()

	return planner.predictor.advance(
		ticker.Symbol,
		ticker.Timestamp,
		reference,
		ticker.Bid.Float64(),
		planner.breakEven(ticker.Symbol),
	)
}

func (planner *Planner) breakEven(symbol string) *float64 {
	cash := planner.desk.Balance().Cash()

	if cash == nil || cash.Sign() <= 0 {
		return nil
	}

	budget := decimal.NewFromInt64(0).Add(cash).Mul(
		decimal.NewFromFloat64(planner.maxAllocationFraction),
	)
	quantity := planner.desk.Price().Quantity(symbol, budget)

	if quantity == nil || quantity.Sign() <= 0 {
		return nil
	}

	executable, err := planner.desk.Price().ExecutableQuantity(symbol, quantity)

	if err != nil || executable == nil || executable.Sign() <= 0 {
		return nil
	}

	cost, err := planner.desk.Price().EntryCost(symbol, executable)

	if err != nil || cost == nil || cost.BreakEven == nil {
		return nil
	}

	value := cost.BreakEven.Float64()

	return &value
}

func (planner *Planner) decide(forecast *directionalForecast) *types.Decision {
	action := types.ActionNothing
	reason := "planner: calibrated probabilities do not clear admission"

	if forecast.probabilityUp >= planner.minimumProbability &&
		forecast.probabilityProfitable >= planner.minimumProbability {
		action = types.ActionEnter
		reason = "planner: upward and executable-profit probabilities clear admission"
	}

	decision := types.NewDecision(action, forecast.symbol)
	decision.At = forecast.at
	decision.Direction = 1
	decision.Confidence = min(forecast.probabilityUp, forecast.probabilityProfitable)
	decision.PredictiveReady = true
	decision.PredictiveStatus = "calibrated-positive-skill"
	decision.TaskSkill = min(forecast.directionSkillLowerBound, forecast.profitSkillLowerBound)
	decision.TaskSkillReady = true
	decision.ForecastSource = "full-observation-direction"
	decision.ForecastModel = "streaming-feature-association-rls-v1"
	decision.Forecast = &forecast.directionOutput
	decision.ForecastHorizon = 1
	decision.CalibrationCount = min(forecast.directionCalibration, forecast.profitCalibration)
	decision.AllocationClass = "none"
	decision.Reason = reason
	decision.Cause = "precursor observations"
	decision.Alternatives = map[string]float64{
		"probability:up":                  forecast.probabilityUp,
		"probability:profitable":          forecast.probabilityProfitable,
		"skill:direction_lower_bound":     forecast.directionSkillLowerBound,
		"skill:profitability_lower_bound": forecast.profitSkillLowerBound,
		"features:direction":              float64(forecast.directionFeatures),
		"features:profitability":          float64(forecast.profitFeatures),
	}

	return decision
}

func (planner *Planner) execute(decision *types.Decision, round *types.StrategyRound) {
	if err := planner.allocation.Calculate([]*types.Decision{decision}); err != nil {
		decision.Action = types.ActionNothing
		decision.Reason = "planner: allocation failed: " + err.Error()
		round.Outcome = "allocation-failed"
		return
	}

	if decision.Action != types.ActionEnter {
		round.Outcome = "admission"
		return
	}

	if err := planner.desk.Execute(*decision); err != nil {
		decision.Action = types.ActionNothing
		decision.Reason = "planner: execution failed: " + err.Error()
		round.Outcome = "execution-failed"
		return
	}

	round.Outcome = "entry"
}

func (planner *Planner) Close() error {
	if planner == nil || planner.cancel == nil {
		return nil
	}

	planner.cancel()

	return nil
}
