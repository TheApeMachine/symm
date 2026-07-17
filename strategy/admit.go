package strategy

import (
	"sort"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/krakenfx/api-go/v2/pkg/spot"
	"github.com/theapemachine/symm/types"
)

/*
admit selects entries by utility while keeping reserved overflow opportunity-
only. Opportunity names may consume normal slots first so the reserve lane
stays available for the next ignition.
*/
func (planner *Planner) admit(
	thesis *types.Thesis,
	entries []types.Decision,
	freeNormal int,
	freeReserved int,
) {
	sort.Slice(entries, func(left, right int) bool {
		return entries[left].Utility > entries[right].Utility
	})

	admittedNormal := 0
	admittedReserved := 0

	for _, decision := range entries {
		opportunity := decision.AllocationClass == "reserved"
		useNormal := admittedNormal < freeNormal
		useReserved := opportunity && admittedReserved < freeReserved

		if !useNormal && !useReserved {
			decision.Action = "nothing"
			decision.Utility = 0

			if !opportunity && freeNormal <= admittedNormal {
				decision.Reason = "normal slots full; reserved requires opportunity"
			}

			if opportunity {
				decision.Reason = "higher-utility entries consumed available slots"
			}

			thesis.Decisions = append(thesis.Decisions, decision)

			continue
		}

		if useNormal {
			admittedNormal++
		}

		if !useNormal && useReserved {
			admittedReserved++
		}

		thesis.Lifecycle.Store(decision.Symbol, types.LifecycleEntrySelected)
		thesis.Decisions = append(thesis.Decisions, decision)
		entryPrice := decimal.NewFromFloat64(
			decision.ReferencePrice * (1 + decision.ExpectedSpread/2),
		)
		thesis.Positions = append(thesis.Positions, types.Holding{
			Symbol: decision.Symbol,
			Qty:    decimal.NewFromFloat64(decision.ProposedQuantity),
			Order: &spot.Order{
				Description: &spot.OrderDescription{
					Pair: decision.Symbol, Type: "enter", OrderType: "market",
				},
				Price:  entryPrice,
				Volume: decimal.NewFromFloat64(decision.ProposedQuantity),
			},
			EntryPrice:    entryPrice,
			Mark:          decimal.NewFromFloat64(decision.ReferencePrice),
			IsOpportunity: opportunity,
		})

		thesis.Orders = append(thesis.Orders, spot.Order{
			Description: &spot.OrderDescription{
				Pair:      decision.Symbol,
				Type:      decision.Action,
				Price:     decimal.NewFromFloat64(decision.ReferencePrice),
				OrderType: "market",
			},
			Volume: decimal.NewFromFloat64(decision.ProposedQuantity),
			Price:  decimal.NewFromFloat64(decision.ReferencePrice),
		})
	}
}

/*
entry computes the complete round-trip utility of opening one slot and caps
proposed capital at the currently visible best-ask capacity. AllocationClass
reserved requires positive OpportunityMargin and CognitiveLead together.
*/
func (planner *Planner) entry(
	thesis *types.Thesis,
	forecast types.Forecasts,
	cognition types.Cognition,
	fee float64,
	capital float64,
) types.Decision {
	proposed := min(capital, forecast.BuyCapacity)
	unitCost := forecast.ReferencePrice * (1 + forecast.ExpectedSpread/2) * (1 + fee)
	quantity := proposed / unitCost
	utility := forecast.ExpectedReturn - 2*fee - forecast.ExpectedSpread -
		forecast.ExpectedImpact - forecast.ExpectedAdverseSelection
	reading := measureOpportunity(forecast, cognition, thesis)

	if proposed <= 0 || utility <= 0 {
		decision := planner.nothing(
			forecast, "expected executable return does not exceed doing nothing",
		)
		decision.Alternatives["enter"] = utility
		decision.ProposedNotional = proposed
		decision.ProposedQuantity = quantity
		decision.ExpectedFees = 2 * fee
		decision.ExpectedSpread = forecast.ExpectedSpread
		decision.OpportunityMargin = reading.Margin
		decision.CognitiveLead = reading.Lead
		decision.BasinConfidence = reading.Basin

		return decision
	}

	allocation := "normal"

	if reading.Reserved() {
		allocation = "reserved"
	}

	return types.Decision{
		Action:            "enter",
		Symbol:            forecast.Symbol,
		At:                forecast.At,
		Utility:           utility,
		Alternatives:      map[string]float64{"enter": utility, "nothing": 0},
		AllocationClass:   allocation,
		ProposedNotional:  proposed,
		ProposedQuantity:  quantity,
		ExpectedFees:      2 * fee,
		ExpectedSpread:    forecast.ExpectedSpread,
		ReferencePrice:    forecast.ReferencePrice,
		ValidThroughEpoch: forecast.ExpiresEpoch,
		ForecastSource:    forecast.Source,
		OpportunityMargin: reading.Margin,
		CognitiveLead:     reading.Lead,
		BasinConfidence:   reading.Basin,
		Cause:             "entry",
		Reason:            "expected executable return exceeds doing nothing",
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
