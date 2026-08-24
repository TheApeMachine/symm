package mcts

import (
	"fmt"
	"math"
	"time"

	"github.com/theapemachine/symm/nomagique/relation"
)

/*
MarketState is the current observational market state: the coordinate values
available at one event time. Only real observations appear here; simulated
states never enter observational history.
*/
type MarketState struct {
	At     time.Time
	Values map[relation.Coordinate]float64
}

/*
MarketModel evolves market state one step forward under the causal
transition model. The log-return is the priced outcome movement of the step;
the noise scale is the transition residual standard deviation. Strategy
actions never directly mutate market state: without an explicit market-impact
model, an action changes portfolio variables only.
*/
type MarketModel interface {
	Step(current MarketState) (next MarketState, logReturn float64, noise float64, err error)
}

/*
CostModel prices action costs in their real units as fractions of executed
notional. Costs enter reward directly; they are never converted into
arbitrary penalty scores.
*/
type CostModel struct {
	// FeeRate is the exchange taker fee fraction (venue fact).
	FeeRate float64
	// SpreadFraction is the fraction of notional crossed on each side.
	SpreadFraction float64
	// SlippageFraction is the modeled slippage fraction on each side
	// (strategy policy; stated explicitly, not disguised as market math).
	SlippageFraction float64
}

/*
TotalFraction returns the per-side cost fraction.
*/
func (costs CostModel) TotalFraction() float64 {
	return costs.FeeRate + costs.SpreadFraction + costs.SlippageFraction
}

/*
PortfolioState is the account state the strategy actually controls.
*/
type PortfolioState struct {
	Cash      float64
	Position  float64
	MarkPrice float64
}

/*
Wealth is the net portfolio wealth: cash plus marked position value.
*/
func (portfolio PortfolioState) Wealth(quantityPerUnit float64) float64 {
	return portfolio.Cash + portfolio.Position*quantityPerUnit*portfolio.MarkPrice
}

/*
EconomicState is one MCTS search state: portfolio state, market state, the
market transition model, costs, and the accumulated economic reward
(change in net wealth). The reward is an actual economic quantity; it is
never a signal or opportunity score.
*/
type EconomicState struct {
	Portfolio   PortfolioState
	Market      MarketState
	MarketModel MarketModel
	Costs       CostModel

	QuantityPerUnit float64
	MaxPosition     float64

	Step        int
	MaxSteps    int
	Accumulated float64
}

/*
NewEconomicState builds the initial search state.
*/
func NewEconomicState(
	portfolio PortfolioState,
	market MarketState,
	marketModel MarketModel,
	costs CostModel,
	quantityPerUnit float64,
	maxPosition float64,
	horizon int,
) *EconomicState {
	if horizon < 1 {
		horizon = 1
	}

	return &EconomicState{
		Portfolio:       portfolio,
		Market:          market,
		MarketModel:     marketModel,
		Costs:           costs,
		QuantityPerUnit: quantityPerUnit,
		MaxPosition:     maxPosition,
		MaxSteps:        horizon,
	}
}

/*
IsTerminal reports whether the horizon is exhausted.
*/
func (state *EconomicState) IsTerminal() bool {
	return state == nil || state.Step >= state.MaxSteps
}

/*
GetPossibleActions returns the feasible interventions. Feasibility is
position reality, exposure policy, and cash coverage only; no evidence
threshold ever gates an action.
*/
func (state *EconomicState) GetPossibleActions() []Action {
	if state == nil || state.IsTerminal() {
		return nil
	}

	if state.Portfolio.Position == 0 {
		if state.affordable(state.QuantityPerUnit) {
			return []Action{Wait, Enter}
		}

		return []Action{Wait}
	}

	actions := []Action{Wait, Exit}

	if state.MaxPosition <= 0 || state.Portfolio.Position < state.MaxPosition {
		if state.affordable(state.QuantityPerUnit) {
			actions = append(actions, Scale)
		}
	}

	return actions
}

/*
affordable reports whether cash covers the notional plus costs of one unit at
the current mark. It is an explicit feasibility constraint (insufficient
funds), not an evidence gate.
*/
func (state *EconomicState) affordable(quantity float64) bool {
	notional := quantity * state.Portfolio.MarkPrice
	return state.Portfolio.Cash >= notional*(1+state.Costs.TotalFraction())
}

/*
ApplyAction evolves the state: the market transitions independently of the
action, then the action mutates only portfolio variables with explicit costs.
The accumulated reward is the exact change in net wealth.
*/
func (state *EconomicState) ApplyAction(action Action) (State, error) {
	if state == nil || state.MarketModel == nil {
		return nil, fmt.Errorf("mcts: economic state and market model are required")
	}

	nextMarket, logReturn, _, err := state.MarketModel.Step(state.Market)

	if err != nil {
		return nil, fmt.Errorf("mcts: market transition failed: %w", err)
	}

	newPrice := state.Portfolio.MarkPrice * math.Exp(logReturn)

	if !(newPrice > 0) {
		return nil, fmt.Errorf("mcts: market transition produced a non-positive price")
	}

	marketDelta := state.Portfolio.Position * state.QuantityPerUnit *
		state.Portfolio.MarkPrice * (math.Exp(logReturn) - 1)

	cash := state.Portfolio.Cash
	position := state.Portfolio.Position
	actionDelta := 0.0

	switch action {
	case Wait:
	case Enter:
		if position != 0 {
			return nil, fmt.Errorf("mcts: enter requires a flat position")
		}

		notional := state.QuantityPerUnit * newPrice
		cost := notional * state.Costs.TotalFraction()

		if cash < notional+cost {
			return nil, fmt.Errorf("mcts: insufficient funds to enter")
		}

		cash -= notional + cost
		position = 1
		actionDelta = -cost
	case Exit:
		if position == 0 {
			return nil, fmt.Errorf("mcts: exit requires an open position")
		}

		notional := position * state.QuantityPerUnit * newPrice
		cost := notional * state.Costs.TotalFraction()
		cash += notional - cost
		position = 0
		actionDelta = -cost
	case Scale:
		if position == 0 {
			return nil, fmt.Errorf("mcts: scale requires an open position")
		}

		if state.MaxPosition > 0 && position+1 > state.MaxPosition {
			return nil, fmt.Errorf("mcts: scale exceeds the exposure cap")
		}

		notional := state.QuantityPerUnit * newPrice
		cost := notional * state.Costs.TotalFraction()

		if cash < notional+cost {
			return nil, fmt.Errorf("mcts: insufficient funds to scale")
		}

		cash -= notional + cost
		position++
		actionDelta = -cost
	default:
		return nil, fmt.Errorf("mcts: unknown action %d", int(action))
	}

	return &EconomicState{
		Portfolio: PortfolioState{
			Cash:      cash,
			Position:  position,
			MarkPrice: newPrice,
		},
		Market:          nextMarket,
		MarketModel:     state.MarketModel,
		Costs:           state.Costs,
		QuantityPerUnit: state.QuantityPerUnit,
		MaxPosition:     state.MaxPosition,
		Step:            state.Step + 1,
		MaxSteps:        state.MaxSteps,
		Accumulated:     state.Accumulated + marketDelta + actionDelta,
	}, nil
}

/*
GetReward returns the accumulated change in net wealth. It is the economic
reward; nothing semantic is added after causal simulation.
*/
func (state *EconomicState) GetReward() float64 {
	if state == nil {
		return 0
	}

	return state.Accumulated
}

/*
State is the search environment contract: feasible actions, transition,
terminal test, and the economic reward.
*/
type State interface {
	IsTerminal() bool
	GetPossibleActions() []Action
	ApplyAction(action Action) (State, error)
	GetReward() float64
}

/*
ActionEstimator supplies the causal economic estimate for one action. It is
the boundary where identification status and model support enter the search.
*/
type ActionEstimator interface {
	EstimateAction(state State, action Action) ActionEstimate
}
