package mcts

import (
	"fmt"
	"math"
	"math/rand"
	"time"

	"github.com/theapemachine/symm/nomagique/relation"
)

/*
MarketSample is one timestamped value of a market coordinate.
*/
type MarketSample struct {
	At    time.Time
	Value float64
}

/*
MarketState is the temporal market state: the coordinate values available at
one event time plus the timestamped trajectory each coordinate carries. The
history is what lets transition evaluation honor a parent's measured lag: the
value valid at t - lag is read as-of, exactly as the Relation and causal
fits were aligned. Only real observations seed the initial state; simulated
values are appended during rollouts and never enter observational history.
*/
type MarketState struct {
	At time.Time
	// Current holds the newest value of each coordinate at At.
	Current map[relation.Coordinate]float64
	// History holds the timestamped trajectory of each coordinate at or
	// before At, newest last. The newest entry per coordinate matches
	// Current when present.
	History map[relation.Coordinate][]MarketSample
}

/*
ValueAt returns the newest value of a coordinate available at or before
At - lag (the as-of value the causal transitions were fitted with). A lag of
zero reads the current value; a positive lag consults the timestamped
history. It reports not-found when no entry satisfies the cutoff — which is
missing, never a fabricated zero.
*/
func (state MarketState) ValueAt(coordinate relation.Coordinate, lag time.Duration) (float64, bool) {
	if lag <= 0 {
		if value, found := state.Current[coordinate]; found {
			return value, true
		}
	}

	cutoff := state.At.Add(-lag)

	if entries := state.History[coordinate]; len(entries) > 0 {
		for index := len(entries) - 1; index >= 0; index-- {
			if !entries[index].At.After(cutoff) {
				return entries[index].Value, true
			}
		}
	}

	return 0, false
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

	EntryPrice        float64
	PeakPrice         float64
	HardFloor         float64
	Floor             float64
	ProfitLine        float64
	ProfitLatched     bool
	StoplossTriggered bool

	observationalHistory [][]float64
}

/*
WithStoplossPolicy configures stoploss geometry for post-entry continuation simulation.
*/
func (state *EconomicState) WithStoplossPolicy(hardFloor, profitLine float64) *EconomicState {
	if state == nil {
		return nil
	}

	state.HardFloor = hardFloor
	state.Floor = hardFloor
	state.ProfitLine = profitLine

	return state
}

/*
WithHistory attaches prior observational rows to the economic state.
*/
func (state *EconomicState) WithHistory(rows [][]float64) *EconomicState {
	if state == nil {
		return nil
	}

	state.observationalHistory = rows
	return state
}

/*
History returns prior observational rows adhering to HistoryProvider.
*/
func (state *EconomicState) History() [][]float64 {
	if state == nil {
		return nil
	}

	return state.observationalHistory
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
	if state == nil || state.IsTerminal() || state.StoplossTriggered {
		return nil
	}

	if state.Portfolio.Position == 0 {
		if state.Step == 0 && state.affordable(state.UnitQuantity) {
			return []Action{Wait, Enter}
		}

		return []Action{Wait}
	}

	// Post-entry MCTS continuation simulates the production Stoploss policy.
	// Production does not allow unconstrained Scale/Exit branching post-entry;
	// the position is held under the simulated Stoploss until triggered or expired.
	return []Action{Wait}
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

	entryPrice := state.EntryPrice
	peakPrice := state.PeakPrice
	hardFloor := state.HardFloor
	floor := state.Floor
	profitLine := state.ProfitLine
	profitLatched := state.ProfitLatched
	stoplossTriggered := state.StoplossTriggered

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

		entryPrice = price
		peakPrice = price

		if hardFloor <= 0 {
			hardFloor = price * (1.0 - 0.02)
		}

		floor = hardFloor

		if profitLine <= 0 {
			profitLine = price * (1.0 + 0.03)
		}
	case Exit:
		if position == 0 {
			return nil, fmt.Errorf("mcts: exit requires an open position")
		}

		notional := position * price
		cost := notional * state.Costs.TotalFraction()
		cash += notional - cost
		position = 0
		actionDelta = -cost
		stoplossTriggered = true
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

	if position > 0 {
		if newPrice > peakPrice {
			peakPrice = newPrice
		}

		if profitLine > 0 && newPrice >= profitLine {
			profitLatched = true

			if entryPrice > floor {
				floor = entryPrice
			}
		}

		if profitLatched && peakPrice > entryPrice {
			trailDist := entryPrice - hardFloor

			if trailDist > 0 && (peakPrice-trailDist) > floor {
				floor = peakPrice - trailDist
			}
		}

		if (floor > 0 && newPrice <= floor) || (hardFloor > 0 && newPrice <= hardFloor) {
			notional := position * newPrice
			exitCost := notional * state.Costs.TotalFraction()
			cash += notional - exitCost
			actionDelta -= exitCost
			position = 0
			stoplossTriggered = true
		}
	}

	return &EconomicState{
		Portfolio: PortfolioState{
			Cash:      cash,
			Position:  position,
			MarkPrice: newPrice,
		},
		Market:               nextMarket,
		MarketModel:          state.MarketModel,
		Costs:                state.Costs,
		UnitQuantity:         state.UnitQuantity,
		MaxPosition:          state.MaxPosition,
		Step:                 state.Step + 1,
		MaxSteps:             state.MaxSteps,
		Accumulated:          state.Accumulated + actionDelta + marketDelta,
		EntryPrice:           entryPrice,
		PeakPrice:            peakPrice,
		HardFloor:            hardFloor,
		Floor:                floor,
		ProfitLine:           profitLine,
		ProfitLatched:        profitLatched,
		StoplossTriggered:    stoplossTriggered,
		observationalHistory: state.observationalHistory,
	}, nil
}

/*
GetReward returns the accumulated change in net wealth. It is the economic
reward; nothing semantic is added after causal simulation.

A rollout that reaches the horizon still holding inventory is charged the full
exit cost (taker fee, spread, and modeled slippage on the held notional) at
the terminal mark: inventory is never free to hold past the horizon. Without
this the search systematically underestimates round-trip friction by the exit
leg, so half-cost rollouts make net-negative trades look profitable and the
planner churns entries that cannot clear their own exit fees.
*/
func (state *EconomicState) GetReward() float64 {
	if state == nil {
		return 0
	}

	if state.IsTerminal() && state.Portfolio.Position > 0 {
		exitNotional := state.Portfolio.Position * state.Portfolio.MarkPrice
		return state.Accumulated - exitNotional*state.Costs.TotalFraction()
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
Vectorizer exports a state as one observational row for the structural model.
The column layout is the state's own contract with the CausalPolicy that names
the target, treatment, control, and feature indices; the search never
interprets the columns itself.

It is optional: a state that cannot vectorize simply contributes no
observational evidence, and the search stays on its observational rung.
*/
type Vectorizer interface {
	ToVector() []float64
}

/*
HistoryProvider supplies the prior observational rows a search starts from,
in the same column layout as Vectorizer. It is optional; without it the
structural model builds its evidence from rollout trajectories alone.
*/
type HistoryProvider interface {
	History() [][]float64
}

/*
ActionEstimator supplies the causal economic estimate for one action. It is
the boundary where identification status and model support enter the search.
*/
type ActionEstimator interface {
	EstimateAction(state State, action Action) ActionEstimate
}

/*
Economic state column layout for the structural model. The treatment is the
signed exposure the action produced, and the target is the wealth change that
followed it, so the interventional query asks the economically meaningful
question: what does holding this much exposure do to net wealth?

The controls are the state variables that confound that relationship — the
price level and how far into the horizon the step occurred — and the features
are what the counterfactual fit regresses on during abduction.
*/
const (
	// EconomicColumnPrice is the mark price the step executed at.
	EconomicColumnPrice = iota
	// EconomicColumnStep is the horizon position of the step.
	EconomicColumnStep
	// EconomicColumnExposure is the post-action position, the treatment.
	EconomicColumnExposure
	// EconomicColumnWealthChange is the accumulated wealth change, the target.
	EconomicColumnWealthChange
	// EconomicColumnWidth is the row width.
	EconomicColumnWidth
)

/*
EconomicCausalPolicy is the CausalPolicy matching EconomicState's row layout.
The support floor and weights remain the caller's declared strategy policy.
*/
func EconomicCausalPolicy(
	minimumRows int,
	expectationWeight float64,
	maxCounterfactualMass float64,
	linearFit bool,
) CausalPolicy {
	// Rejection stays disarmed here: a floor is a risk decision in reward
	// units, so the caller arms it explicitly with WithRejectionFloor rather
	// than inheriting a default that happens to be zero.
	return CausalPolicy{
		TargetColumn:    EconomicColumnWealthChange,
		TreatmentColumn: EconomicColumnExposure,
		ControlColumns:  []int{EconomicColumnPrice, EconomicColumnStep},
		FeatureColumns: []int{
			EconomicColumnPrice,
			EconomicColumnStep,
			EconomicColumnExposure,
		},
		MinimumRows:           minimumRows,
		LinearFit:             linearFit,
		ExpectationWeight:     expectationWeight,
		MaxCounterfactualMass: maxCounterfactualMass,
	}
}

/*
ToVector exports the state as one observational row in the layout the
EconomicCausalPolicy columns name.
*/
func (state *EconomicState) ToVector() []float64 {
	if state == nil {
		return nil
	}

	row := make([]float64, EconomicColumnWidth)
	row[EconomicColumnPrice] = state.Portfolio.MarkPrice
	row[EconomicColumnStep] = float64(state.Step)
	row[EconomicColumnExposure] = state.Portfolio.Position
	row[EconomicColumnWealthChange] = state.Accumulated

	return row
}

/*
GetInterventionLevel names the treatment level each action represents: the
exposure the portfolio would carry after taking it. An action that is not
feasible from this state has no level, because intervening at an exposure the
position could never reach would ask the model an unanswerable question.
*/
func (state *EconomicState) GetInterventionLevel(action Action) (float64, bool) {
	if state == nil {
		return 0, false
	}

	position := state.Portfolio.Position

	switch action {
	case Wait:
		return position, true
	case Enter:
		if position != 0 {
			return 0, false
		}

		return state.UnitQuantity, true
	case Exit:
		if position == 0 {
			return 0, false
		}

		return 0, true
	case Scale:
		if position == 0 {
			return 0, false
		}

		return position + state.UnitQuantity, true
	default:
		return 0, false
	}
}

/*
WithRejectionFloor arms causal rejection at an explicit floor, in the same
units as the economic reward. It is a separate call because zero is itself a
meaningful floor (reject any action whose causal effect on wealth is negative)
and must not be indistinguishable from an unconfigured policy.
*/
func (policy CausalPolicy) WithRejectionFloor(floor float64) CausalPolicy {
	policy.RejectionFloor = floor
	policy.RejectionEnabled = true

	return policy
}
