package strategy

import (
	"context"
	"sort"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/krakenfx/api-go/v2/pkg/spot"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/logic"
	"github.com/theapemachine/symm/types"
)

/*
Planner records the feasible action alternatives for each calibrated forecast
and emits orders only for actions that cross the broker boundary.
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
	fees map[string]float64,
	available float64,
	slots int,
) *types.Thesis {
	entries := make([]types.Decision, 0)
	openBySymbol := make(map[string]types.Holding, len(thesis.Positions))

	for _, holding := range thesis.Positions {
		if holding.Order == nil || holding.Order.Description == nil {
			continue
		}

		openBySymbol[holding.Order.Description.Pair] = holding
	}

	remainingSlots := slots - len(openBySymbol)

	for _, forecast := range thesis.Forecasts {
		fee, feeReady := fees[forecast.Symbol]

		if !forecast.Eligible() || !feeReady || fee < 0 {
			continue
		}

		if _, ok := thesis.Lifecycle.Load(forecast.Symbol); !ok {
			thesis.Lifecycle.Store(forecast.Symbol, types.LifecycleShaped)
		}

		if holding, exists := openBySymbol[forecast.Symbol]; exists {
			decision := planner.continuation(forecast, fee, holding)
			decision.Cause = planner.cause(thesis, forecast, decision.Action)
			planner.context(&decision, forecast, available, len(openBySymbol), slots)
			thesis.Decisions = append(thesis.Decisions, decision)

			if decision.Action == "exit" {
				thesis.Lifecycle.Store(forecast.Symbol, types.LifecycleExitSelected)
			}

			if decision.Action == "exit" || decision.Action == "reduce" {
				thesis.Orders = append(thesis.Orders, spot.Order{
					Description: &spot.OrderDescription{
						Pair:      forecast.Symbol,
						Type:      decision.Action,
						Price:     decimal.NewFromFloat64(decision.ReferencePrice),
						OrderType: "market",
					},
					Volume: decimal.NewFromFloat64(decision.ProposedQuantity),
					Price:  decimal.NewFromFloat64(decision.ReferencePrice),
				})
			}

			continue
		}

		referencePrice := decimal.NewFromFloat64(forecast.ReferencePrice)
		entryPrice := decimal.NewFromFloat64(
			forecast.ReferencePrice * (1 + forecast.ExpectedSpread/2),
		)
		candidateIndex := len(thesis.Positions)
		thesis.Positions = append(thesis.Positions, types.Holding{
			Symbol: forecast.Symbol,
			Qty:    *decimal.NewFromInt64(0),
			Order: &spot.Order{
				Description: &spot.OrderDescription{
					Pair: forecast.Symbol, Type: "enter", OrderType: "market",
				},
				Price: entryPrice,
			},
			EntryPrice: *entryPrice,
			Mark:       *referencePrice,
		})

		cognitionValue, cognitionFound := thesis.Cognition.Load(forecast.Symbol)
		cognition, cognitionValid := cognitionValue.(types.Cognition)

		if !cognitionFound || !cognitionValid || !cognition.Ready ||
			cognition.Ambiguous || cognition.Winner != "buy" {
			reason := "cognitive memory is not ready for this evidence sequence"
			cause := "cognitive_not_ready"

			if cognitionValid && cognition.Ambiguous {
				reason = "cognitive memory is ambiguous for this evidence sequence"
				cause = "cognitive_ambiguity"
			}

			if cognitionValid && cognition.Ready && !cognition.Ambiguous &&
				cognition.Winner != "buy" {
				reason = "cognitive memory does not support a buy entry"
				cause = "cognitive_opposition"
			}

			decision := planner.nothing(forecast, reason)
			decision.Cause = cause
			planner.context(&decision, forecast, available, len(openBySymbol), slots)
			thesis.Decisions = append(thesis.Decisions, decision)

			continue
		}

		if remainingSlots <= 0 || available <= 0 {
			decision := planner.nothing(
				forecast, "portfolio capacity makes entry infeasible",
			)
			planner.context(&decision, forecast, available, len(openBySymbol), slots)
			thesis.Decisions = append(thesis.Decisions, decision)

			continue
		}

		decision := planner.entry(
			forecast,
			fee,
			available/float64(remainingSlots),
		)
		planner.context(&decision, forecast, available, len(openBySymbol), slots)
		candidate := &thesis.Positions[candidateIndex]
		candidate.Qty = *decimal.NewFromFloat64(
			decision.ProposedNotional / candidate.EntryPrice.Float64(),
		)
		candidate.Order.Volume = &candidate.Qty

		if decision.Action == "nothing" {
			thesis.Decisions = append(thesis.Decisions, decision)

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
			thesis.Decisions = append(thesis.Decisions, decision)

			continue
		}

		thesis.Lifecycle.Store(decision.Symbol, types.LifecycleEntrySelected)
		thesis.Decisions = append(thesis.Decisions, decision)

		thesis.Orders = append(thesis.Orders, spot.Order{
			Description: &spot.OrderDescription{
				Pair:      decision.Symbol,
				Type:      decision.Action,
				Price:     decimal.NewFromFloat64(decision.ReferencePrice),
				OrderType: "market",
			},
			Volume: decimal.NewFromFloat64(decision.ProposedNotional),
			Price:  decimal.NewFromFloat64(decision.ReferencePrice),
		})
	}

	return thesis
}

/*
continuation computes hold and exit utility from the same current forecast.
Entry cost is sunk; exiting now pays one fee and one side of the spread.
*/
func (planner *Planner) continuation(
	forecast types.Forecasts,
	fee float64,
	holding types.Holding,
) types.Decision {
	hold := forecast.ExpectedReturn
	exit := -(fee + forecast.ExpectedSpread/2)
	action := "hold"
	utility := hold
	reason := "remaining expected return exceeds current exit cost"
	alternatives := map[string]float64{"hold": hold}
	quantity := 0.0
	mark := holding.Mark.Float64()
	qty := holding.Qty.Float64()
	notional := mark * qty

	exitAvailable := notional > 0 && forecast.SellCapacity >= notional

	if exitAvailable {
		alternatives["exit"] = exit
	}

	if exitAvailable && exit > hold {
		action = "exit"
		utility = exit
		quantity = qty
		reason = "current exit utility exceeds remaining expected return"
	}

	if notional > forecast.SellCapacity && notional > 0 {
		fraction := forecast.SellCapacity / notional
		reduce := fraction*exit + (1-fraction)*hold
		alternatives["reduce"] = reduce

		if reduce > utility {
			action = "reduce"
			utility = reduce
			quantity = qty * fraction
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
Update measures and analyzes the next Thesis before portfolio context is applied.
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
