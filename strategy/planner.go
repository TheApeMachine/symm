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
Planner is the sole live entry authority. It asks the adaptive executable-return
distribution whether entering dominates keeping cash for the current
opportunity. Desk-owned Stoploss instances retain exclusive exit authority.
*/
type Planner struct {
	cancel                context.CancelFunc
	desk                  *broker.Desk
	predictor             *directionalPredictor
	allocation            *Allocation
	recorder              *audit.Recorder
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
		initialVariance:  viper.GetFloat64("market.forecast.rls.initial_variance"),
		forgettingFactor: viper.GetFloat64("market.forecast.rls.forgetting_factor"),
	})

	if err != nil {
		return nil, err
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

	decision, round := planner.preDecision(forecast)

	if forecast.ready {
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
	decision := types.NewDecision(types.ActionNothing, forecast.symbol)
	decision.At = forecast.at
	decision.Direction = 1
	decision.PredictiveReady = false
	decision.PredictiveStatus = forecast.status
	decision.TaskSkillReady = false
	decision.ForecastSource = "opportunity-conditioned-observations-and-perspectives"
	decision.ForecastModel = "adaptive-executable-return-student-t-v1"
	decision.Forecast = &forecast.output
	decision.ForecastHorizon = forecast.horizonSteps
	decision.CalibrationCount = forecast.calibration
	decision.AllocationClass = "none"
	decision.Reason = "planner: " + forecast.status
	decision.Cause = "opportunity-conditioned market context"
	decision.Opportunity = forecast.opportunity.Archetype != ""
	decision.OpportunityType = string(forecast.opportunity.Archetype)
	decision.OpportunityPhase = string(forecast.opportunity.Phase)
	decision.Alternatives = map[string]float64{
		"probability:up":             forecast.probabilityUp,
		"probability:profitable":     forecast.probabilityProfitable,
		"return:expected_log":        forecast.expectedLogReturn,
		"return:break_even_log":      forecast.breakEvenLogReturn,
		"return:scale":               forecast.output.Scale,
		"return:degrees_of_freedom":  forecast.output.DegreesOfFreedom,
		"horizon:seconds":            forecast.horizon.Seconds(),
		"features:directional":       float64(forecast.directionalFeatures),
		"features:estimability":      float64(forecast.estimabilityFeatures),
		"features:execution_context": float64(forecast.executionFeatures),
		"features:semantic_review":   float64(forecast.reviewFeatures),
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

	breakEven, err := planner.breakEven(ticker.Symbol)

	if err != nil {
		return nil, err
	}

	return planner.predictor.advance(
		ticker.Symbol,
		ticker.Timestamp,
		ticker.Bid.Float64(),
		breakEven,
	)
}

func (planner *Planner) breakEven(symbol string) (*float64, error) {
	cash := planner.desk.Balance().Cash()

	if cash == nil || cash.Sign() <= 0 {
		return nil, fmt.Errorf("planner: positive quote cash required to price entry")
	}

	budget := decimal.NewFromInt64(0).Add(cash).Mul(
		decimal.NewFromFloat64(planner.maxAllocationFraction),
	)
	quantity := planner.desk.Price().Quantity(symbol, budget)

	if quantity == nil || quantity.Sign() <= 0 {
		return nil, fmt.Errorf("planner: positive proposed quantity required to price entry")
	}

	executable, err := planner.desk.Price().ExecutableQuantity(symbol, quantity)

	if err != nil {
		return nil, fmt.Errorf("planner: executable proposed quantity required: %w", err)
	}

	if executable == nil || executable.Sign() <= 0 {
		return nil, fmt.Errorf("planner: executable proposed quantity required")
	}

	cost, err := planner.desk.Price().EntryCost(symbol, executable)

	if err != nil {
		return nil, fmt.Errorf("planner: fee-inclusive entry boundary required: %w", err)
	}

	if cost == nil || cost.BreakEven == nil {
		return nil, fmt.Errorf("planner: fee-inclusive entry boundary required")
	}

	value := cost.BreakEven.Float64()

	return &value, nil
}

func (planner *Planner) decide(forecast *directionalForecast) *types.Decision {
	action := types.ActionNothing
	reason := "planner: keeping cash dominates the forecast entry distribution"

	if forecast.expectedLogReturn > forecast.breakEvenLogReturn {
		action = types.ActionEnter
		reason = "planner: forecast executable return dominates fee-inclusive break-even"
	}

	decision := types.NewDecision(action, forecast.symbol)
	decision.At = forecast.at
	decision.Direction = 1
	decision.Confidence = forecast.probabilityProfitable
	decision.PredictiveReady = true
	decision.PredictiveStatus = forecast.status
	decision.ForecastSource = "opportunity-conditioned-observations-and-perspectives"
	decision.ForecastModel = "adaptive-executable-return-student-t-v1"
	decision.Forecast = &forecast.output
	decision.ForecastHorizon = forecast.horizonSteps
	decision.CalibrationCount = forecast.calibration
	decision.AllocationClass = "none"
	decision.Reason = reason
	decision.Cause = "opportunity-conditioned market context"
	decision.Opportunity = true
	decision.OpportunityType = string(forecast.opportunity.Archetype)
	decision.OpportunityPhase = string(forecast.opportunity.Phase)
	decision.Alternatives = map[string]float64{
		"probability:up":             forecast.probabilityUp,
		"probability:profitable":     forecast.probabilityProfitable,
		"return:expected_log":        forecast.expectedLogReturn,
		"return:break_even_log":      forecast.breakEvenLogReturn,
		"return:scale":               forecast.output.Scale,
		"return:degrees_of_freedom":  forecast.output.DegreesOfFreedom,
		"horizon:seconds":            forecast.horizon.Seconds(),
		"features:directional":       float64(forecast.directionalFeatures),
		"features:estimability":      float64(forecast.estimabilityFeatures),
		"features:execution_context": float64(forecast.executionFeatures),
		"features:semantic_review":   float64(forecast.reviewFeatures),
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
