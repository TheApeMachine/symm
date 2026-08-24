package mcts

import (
	"fmt"
	"math"
	"math/rand"
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
the noise is the transition residual scale of the priced outcome.

The random source, when non-nil, samples the transition noise so rollouts
walk a distribution of plausible causal trajectories: expected movement plus
residual noise, not one deterministic expected path. Strategy actions never
directly mutate market state: without an explicit market-impact model, an
action changes portfolio variables only.
*/
type MarketModel interface {
	Step(current MarketState, random *rand.Rand) (next MarketState, logReturn float64, noise float64, err error)
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
PortfolioState is the account state the strategy actually controls. Position
is the actual held base quantity (for a held lot) or zero; it is never a lot
count.
*/
type PortfolioState struct {
	Cash      float64
	Position  float64
	MarkPrice float64
}

/*
Wealth is the net portfolio wealth: cash plus marked position value.
*/
func (portfolio PortfolioState) Wealth() float64 {
	return portfolio.Cash + portfolio.Position*portfolio.MarkPrice
}

/*
EconomicState is one MCTS search state: portfolio state (position in base
units), market state, the market transition model, costs, and the accumulated
economic reward (change in net wealth). The reward is an actual economic
quantity; it is never a signal or opportunity score.

Action timing is: at time t, the action is executed at the current price,
then the market evolves to t+1 and the post-action exposure earns or loses
the move. An Enter therefore participates in the first forecasted move; an
Exit stops exposure before it.
*/
type EconomicState struct {
	Portfolio   PortfolioState
	Market      MarketState
	MarketModel MarketModel
	Costs       CostModel

	// UnitQuantity is the base quantity one position unit represents
	// (entry sizing policy). Position is expressed in base units.
	UnitQuantity float64
	// MaxPosition is the maximum base quantity the exposure policy allows.
	MaxPosition float64

	Step        int
	MaxSteps    int
	Accumulated float64
}

/*
NewEconomicState builds the initial search state. Position is the actual
held base quantity (0 when flat). UnitQuantity is the sized base quantity one
new position unit represents.
*/
func NewEconomicState(
	portfolio PortfolioState,
	market MarketState,
	marketModel MarketModel,
	costs CostModel,
	unitQuantity float64,
	maxPosition float64,
	horizon int,
) *EconomicState {
	if horizon < 1 {
		horizon = 1
	}

	return &EconomicState{
		Portfolio:    portfolio,
		Market:       market,
		MarketModel:  marketModel,
		Costs:        costs,
		UnitQuantity: unitQuantity,
		MaxPosition:  maxPosition,
		MaxSteps:     horizon,
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
		if state.affordable(state.UnitQuantity) {
			return []Action{Wait, Enter}
		}

		return []Action{Wait}
	}

	actions := []Action{Wait, Exit}

	if state.MaxPosition <= 0 || state.Portfolio.Position+state.UnitQuantity <= state.MaxPosition {
		if state.affordable(state.UnitQuantity) {
			actions = append(actions, Scale)
		}
	}

	return actions
}

/*
affordable reports whether cash covers the notional plus costs of a quantity
at the current mark. It is an explicit feasibility constraint (insufficient
funds), not an evidence gate. Because the action executes at the current
price, this check is exact.
*/
func (state *EconomicState) affordable(quantity float64) bool {
	notional := quantity * state.Portfolio.MarkPrice
	return state.Portfolio.Cash >= notional*(1+state.Costs.TotalFraction())
}

/*
ApplyAction evolves the state: the action is executed at the current price
(mutating only portfolio variables with explicit costs), then the market
transitions to the next time slice. The accumulated reward is the exact
change in net wealth: the action's cost plus the post-action exposure's
realized market move. The random source, when non-nil, samples the market
transition noise.
*/
func (state *EconomicState) ApplyAction(action Action, random *rand.Rand) (State, error) {
	if state == nil || state.MarketModel == nil {
		return nil, fmt.Errorf("mcts: economic state and market model are required")
	}

	price := state.Portfolio.MarkPrice
	cash := state.Portfolio.Cash
	position := state.Portfolio.Position
	actionDelta := 0.0

	switch action {
	case Wait:
	case Enter:
		if position != 0 {
			return nil, fmt.Errorf("mcts: enter requires a flat position")
		}

		notional := state.UnitQuantity * price
		cost := notional * state.Costs.TotalFraction()

		if cash < notional+cost {
			return nil, fmt.Errorf("mcts: insufficient funds to enter")
		}

		cash -= notional + cost
		position = state.UnitQuantity
		actionDelta = -cost
	case Exit:
		if position == 0 {
			return nil, fmt.Errorf("mcts: exit requires an open position")
		}

		notional := position * price
		cost := notional * state.Costs.TotalFraction()
		cash += notional - cost
		position = 0
		actionDelta = -cost
	case Scale:
		if position == 0 {
			return nil, fmt.Errorf("mcts: scale requires an open position")
		}

		if state.MaxPosition > 0 && position+state.UnitQuantity > state.MaxPosition {
			return nil, fmt.Errorf("mcts: scale exceeds the exposure cap")
		}

		notional := state.UnitQuantity * price
		cost := notional * state.Costs.TotalFraction()

		if cash < notional+cost {
			return nil, fmt.Errorf("mcts: insufficient funds to scale")
		}

		cash -= notional + cost
		position += state.UnitQuantity
		actionDelta = -cost
	default:
		return nil, fmt.Errorf("mcts: unknown action %d", int(action))
	}

	nextMarket, logReturn, _, err := state.MarketModel.Step(state.Market, random)

	if err != nil {
		return nil, fmt.Errorf("mcts: market transition failed: %w", err)
	}

	newPrice := price * math.Exp(logReturn)

	if !(newPrice > 0) || math.IsNaN(newPrice) || math.IsInf(newPrice, 0) {
		return nil, fmt.Errorf("mcts: market transition produced a non-positive or non-finite price")
	}

	marketDelta := position * (newPrice - price)

	return &EconomicState{
		Portfolio: PortfolioState{
			Cash:      cash,
			Position:  position,
			MarkPrice: newPrice,
		},
		Market:       nextMarket,
		MarketModel:  state.MarketModel,
		Costs:        state.Costs,
		UnitQuantity: state.UnitQuantity,
		MaxPosition:  state.MaxPosition,
		Step:         state.Step + 1,
		MaxSteps:     state.MaxSteps,
		Accumulated:  state.Accumulated + actionDelta + marketDelta,
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
	ApplyAction(action Action, random *rand.Rand) (State, error)
	GetReward() float64
}

/*
ActionEstimator supplies the causal economic estimate for one action. It is
the boundary where identification status and model support enter the search.
*/
type ActionEstimator interface {
	EstimateAction(state State, action Action) ActionEstimate
}
