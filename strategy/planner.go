package strategy

import (
	"context"
	"sort"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/logic"
	"github.com/theapemachine/symm/types"
)

/*
Planner records the feasible action alternatives for each calibrated forecast
and emits Intents only for actions that cross the broker boundary.
*/
type Planner struct {
	ctx      context.Context
	cancel   context.CancelFunc
	status   types.Status
	uiHub    chan<- []byte
	signals  []types.Signal
	analyzer *logic.Analyzer
}

/*
Decide compares current executable utility for exposed and unexposed symbols.
Entries compete with doing nothing; open positions compare current continuation
with paying the observable cost to exit now.
*/
func (planner *Planner) Decide(
	thesis *types.Thesis,
	positions map[string]types.Exposure,
	fees map[string]float64,
	available float64,
	slots int,
) []Intent {
	intents := make([]Intent, 0)
	entries := make([]types.Decision, 0)
	remainingSlots := slots - len(positions)

	for _, forecast := range thesis.Forecasts {
		fee, feeReady := fees[forecast.Symbol]

		if !forecast.Eligible() || !feeReady || fee < 0 {
			continue
		}

		if thesis.LifecycleState(forecast.Symbol) == types.LifecycleObserving {
			if err := thesis.Transition(
				forecast.Symbol, types.LifecycleShaped, forecast.At,
			); err != nil {
				errnie.Error(err)
				continue
			}
		}

		if exposure, exists := positions[forecast.Symbol]; exists {
			lifecycle := exposure.Thesis

			if lifecycle.LifecycleState(forecast.Symbol) == types.LifecycleInvalid {
				continue
			}

			lifecycle.Absorb(thesis, forecast.Symbol)

			if lifecycle.LifecycleState(forecast.Symbol) == types.LifecycleEntered {
				if err := lifecycle.Transition(
					forecast.Symbol, types.LifecycleManaging, forecast.At,
				); err != nil {
					errnie.Error(err)
					continue
				}
			}

			decision := planner.continuation(forecast, fee, exposure)
			decision.Cause = planner.cause(lifecycle, forecast, decision.Action)
			planner.context(&decision, forecast, available, len(positions), slots)

			if decision.Action == "exit" {
				if err := lifecycle.Transition(
					forecast.Symbol, types.LifecycleExitSelected, forecast.At,
				); err != nil {
					errnie.Error(err)
					continue
				}
			}

			index := lifecycle.RecordDecision(decision)

			if decision.Action == "exit" || decision.Action == "reduce" {
				intents = append(intents, Intent{Thesis: lifecycle, Decision: index})
			}

			continue
		}

		if remainingSlots <= 0 || available <= 0 {
			decision := planner.nothing(
				forecast, "portfolio capacity makes entry infeasible",
			)
			planner.context(&decision, forecast, available, len(positions), slots)
			thesis.RecordDecision(decision)

			continue
		}

		decision := planner.entry(
			forecast,
			fee,
			available/float64(remainingSlots),
		)
		planner.context(&decision, forecast, available, len(positions), slots)

		if decision.Action == "nothing" {
			thesis.RecordDecision(decision)

			continue
		}

		entries = append(entries, decision)
	}

	sort.Slice(entries, func(left, right int) bool {
		return entries[left].Utility > entries[right].Utility
	})

	selected := min(len(entries), max(remainingSlots, 0))

	for index, decision := range entries {
		if index >= selected {
			decision.Action = "nothing"
			decision.Utility = 0
			decision.Reason = "higher-utility entries consumed available slots"
			thesis.RecordDecision(decision)

			continue
		}

		if err := thesis.Transition(
			decision.Symbol, types.LifecycleEntrySelected, decision.At,
		); err != nil {
			errnie.Error(err)
			continue
		}

		index := thesis.RecordDecision(decision)
		intents = append(intents, Intent{Thesis: thesis, Decision: index})
	}

	return intents
}

/*
continuation computes hold and exit utility from the same current forecast.
Entry cost is sunk; exiting now pays one fee and one side of the spread.
*/
func (planner *Planner) continuation(
	forecast types.Forecasts,
	fee float64,
	exposure types.Exposure,
) types.Decision {
	hold := forecast.ExpectedReturn
	exit := -(fee + forecast.ExpectedSpread/2)
	action := "hold"
	utility := hold
	reason := "remaining expected return exceeds current exit cost"
	alternatives := map[string]float64{"hold": hold}
	quantity := 0.0

	exitAvailable := exposure.Notional > 0 && forecast.SellCapacity >= exposure.Notional

	if exitAvailable {
		alternatives["exit"] = exit
	}

	if exitAvailable && exit > hold {
		action = "exit"
		utility = exit
		quantity = exposure.Quantity
		reason = "current exit utility exceeds remaining expected return"
	}

	if exposure.Notional > forecast.SellCapacity && exposure.Notional > 0 {
		fraction := forecast.SellCapacity / exposure.Notional
		reduce := fraction*exit + (1-fraction)*hold
		alternatives["reduce"] = reduce

		if reduce > utility {
			action = "reduce"
			utility = reduce
			quantity = exposure.Quantity * fraction
			reason = "visible bid capacity supports reduction but not complete exit"
		}
	}

	return types.Decision{
		Action:            action,
		Symbol:            forecast.Symbol,
		At:                forecast.At,
		Utility:           utility,
		Alternatives:      alternatives,
		ProposedQuantity:  quantity,
		ExpectedFees:      fee,
		ExpectedSpread:    forecast.ExpectedSpread / 2,
		ReferencePrice:    forecast.ReferencePrice,
		ValidThroughEpoch: forecast.ExpiresEpoch,
		ForecastSource:    forecast.Source,
		Cause:             "continuation",
		Reason:            reason,
	}
}

/*
cause identifies the evidence boundary behind a management action. A ready
negative causal outcome is opposing-thesis formation; an elapsed entry forecast
is invalidation; a negative current forecast without either is weakening.
*/
func (planner *Planner) cause(
	thesis *types.Thesis,
	forecast types.Forecasts,
	action string,
) string {
	if action == "hold" {
		return "continuation"
	}

	if action == "reduce" {
		return "liquidity_deterioration"
	}

	for index := len(thesis.Hypotheses) - 1; index >= 0; index-- {
		hypothesis := thesis.Hypotheses[index]

		if hypothesis.Symbol == forecast.Symbol && hypothesis.Ready &&
			hypothesis.Outcome == forecast.Target && hypothesis.DoExpectation < 0 &&
			hypothesis.Uplift < 0 {
			return "opposing_thesis"
		}
	}

	for index := len(thesis.Decisions) - 1; index >= 0; index-- {
		decision := thesis.Decisions[index]

		if decision.Symbol == forecast.Symbol && decision.Action == "enter" &&
			forecast.SourceEpoch >= decision.ValidThroughEpoch {
			return "thesis_invalidation"
		}
	}

	return "thesis_weakening"
}

/*
entry computes the complete round-trip utility of opening one normal slot and
caps proposed capital at the currently visible best-ask capacity.
*/
func (planner *Planner) entry(
	forecast types.Forecasts,
	fee float64,
	capital float64,
) types.Decision {
	proposed := min(capital, forecast.BuyCapacity)
	utility := forecast.ExpectedReturn - 2*fee - forecast.ExpectedSpread -
		forecast.ExpectedImpact - forecast.ExpectedAdverseSelection

	if proposed <= 0 || utility <= 0 {
		decision := planner.nothing(
			forecast, "expected executable return does not exceed doing nothing",
		)
		decision.Alternatives["enter"] = utility
		decision.ProposedNotional = proposed
		decision.ExpectedFees = 2 * fee
		decision.ExpectedSpread = forecast.ExpectedSpread

		return decision
	}

	return types.Decision{
		Action: "enter", Symbol: forecast.Symbol, At: forecast.At,
		Utility: utility, Alternatives: map[string]float64{"enter": utility, "nothing": 0},
		AllocationClass: "normal", ProposedNotional: proposed,
		ExpectedFees: 2 * fee, ExpectedSpread: forecast.ExpectedSpread,
		ReferencePrice: forecast.ReferencePrice, ValidThroughEpoch: forecast.ExpiresEpoch,
		ForecastSource: forecast.Source, Cause: "entry",
		Reason: "expected executable return exceeds doing nothing",
	}
}

/*
nothing records an explicit no-action selection while retaining the forecast
price and validity boundary that made the comparison possible.
*/
func (planner *Planner) nothing(
	forecast types.Forecasts,
	reason string,
) types.Decision {
	return types.Decision{
		Action:            "nothing",
		Symbol:            forecast.Symbol,
		At:                forecast.At,
		Alternatives:      map[string]float64{"nothing": 0},
		ReferencePrice:    forecast.ReferencePrice,
		ValidThroughEpoch: forecast.ExpiresEpoch,
		ForecastSource:    forecast.Source,
		Cause:             "infeasible",
		Reason:            reason,
	}
}

/*
context records the forecast decomposition and portfolio values actually used
for one utility comparison so the Decision remains auditable on its Thesis.
*/
func (planner *Planner) context(
	decision *types.Decision,
	forecast types.Forecasts,
	available float64,
	openPositions int,
	slots int,
) {
	decision.ForecastModel = forecast.ModelVersion
	decision.ForecastEpoch = forecast.SourceEpoch
	decision.CalibrationCount = forecast.CalibrationSamples
	decision.ExpectedReturn = forecast.ExpectedReturn
	decision.ExpectedImpact = forecast.ExpectedImpact
	decision.AdverseSelection = forecast.ExpectedAdverseSelection
	decision.Uncertainty = forecast.Uncertainty
	decision.Confidence = forecast.Confidence
	decision.AvailableCapital = available
	decision.OpenPositions = openPositions
	decision.SlotCapacity = slots
}

/*
NewPlanner creates a Planner that is ready once its dependencies are assigned.
Planning has no deferred initialization or warmup of its own.
*/
func NewPlanner(
	ctx context.Context,
	uiHub chan<- []byte,
	signals []types.Signal,
	analyzer *logic.Analyzer,
) *Planner {
	ctx, cancel := context.WithCancel(ctx)

	return &Planner{
		ctx:      ctx,
		cancel:   cancel,
		status:   types.READY,
		uiHub:    uiHub,
		signals:  signals,
		analyzer: analyzer,
	}
}

func (planner *Planner) Initialize() error {
	errnie.Info("initializing planner")
	planner.status = types.READY
	return nil
}

/*
Status reports whether the Planner itself is ready to evaluate evidence.
Boot-stage admission remains a separate concern enforced by Update.
*/
func (planner *Planner) Status() types.Status {
	return planner.status
}

/*
Update evaluates the thesis for all symbols and returns intended actions.
*/
func (planner *Planner) Update() *types.Thesis {
	thesis := types.NewThesis(planner.uiHub)

	for _, signal := range planner.signals {
		thesis = signal.Measure(thesis)
	}

	if planner.analyzer != nil {
		planner.analyzer.Update(thesis)
	}

	return thesis
}
