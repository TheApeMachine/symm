package strategy

import (
	"sync"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/nomagique/mcts"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/logic/graph"
	"github.com/theapemachine/symm/types"
)

type Evaluator struct {
	mctsEngine *mcts.CausalMCTS
	desk       *broker.Desk
	price      *broker.Price
	balance    *broker.Balance
}

func NewEvaluator(
	desk *broker.Desk,
	price *broker.Price,
	balance *broker.Balance,
) Evaluator {
	engine := NewCausalEngineAdapter()
	// Configure MCTS: Exploration C = 1.414, CausalAlpha = 0.5
	// Target = 3 (Reward), Treatment = 2 (Action), Controls = [0, 1] (Energy, Surprise)
	mctsSearch := mcts.NewCausalMCTS(
		engine,
		1.414, 0.5,
		12, 2, 3,
		[]int{0, 1}, []int{0, 1, 2},
		false, // Non-linear SCM fit
	)

	return Evaluator{
		mctsEngine: mctsSearch,
		desk:       desk,
		price:      price,
		balance:    balance,
	}
}

func (evaluator Evaluator) EvaluateOpportunities(thesis *types.Thesis) {
	if thesis == nil {
		return
	}

	if thesis.Cognition == nil {
		thesis.Cognition = &sync.Map{}
	}

	if thesis.Causal == nil {
		thesis.Causal = &sync.Map{}
	}

	if thesis.Graphs == nil {
		thesis.Graphs = &sync.Map{}
	}

	occupied := getOccupiedSymbols(thesis, evaluator.desk)
	forecasts := selectForecasts(thesis.Forecasts)

	for symbol, forecast := range forecasts {
		if _, blocked := occupied[symbol]; blocked {
			continue
		}

		if isExiting(thesis, symbol) {
			continue
		}

		if !forecast.FrictionReady {
			evaluator.stampFriction(&forecast)
		}

		if !forecast.Eligible() {
			continue
		}

		// 1. Extract Relational Graph & Causal Rows
		supports, contradicts, hasGraph := inspectGraph(thesis, symbol)
		causalRows := getCausalHistoryRows(thesis, symbol)
		doExpectation, uplift, noise, causalReady := getCausalMetrics(thesis, symbol)
		cognition := getCognition(thesis, symbol)

		if hasGraph && contradicts > supports+1.0 {
			thesis.Decisions = append(thesis.Decisions, evaluator.reject(
				forecast, 0, "graph_contradiction",
				"relational graph contradicts trade hypothesis",
			))
			continue
		}

		// 2. Build Root State for MCTS
		rootState := StrategyState{
			Symbol:    symbol,
			Energy:    forecast.Uncertainty,
			Surprise:  forecast.ExpectedImpact.Float64(),
			Treatment: forecast.ExpectedReturn.Float64(),
			Reward:    0.0,
			Step:      0,
			MaxSteps:  5, // 5-step forward trajectory search
			IsHolding: false,
		}

		// 3. Run Causal MCTS Search over Trajectory Tree
		var recommendedAction float64
		var mctsErr error

		if len(causalRows) >= 12 {
			recommendedAction, mctsErr = evaluator.mctsEngine.Search(rootState, 50, causalRows)
		}

		// 4. Executable Net Return
		execReturn := forecast.ExecutableReturn()
		utility := execReturn.Float64() - forecast.Uncertainty

		if causalReady {
			utility += doExpectation + uplift - noise
		}

		if cognition.Ready {
			utility += cognition.LookaheadScore * cognition.Confidence
		}

		// If MCTS recommends ActionEnter (1.0) and Net Utility > 0
		if (mctsErr == nil && recommendedAction == ActionEnter) || (len(causalRows) < 12 && utility > 0) {
			thesis.Decisions = append(thesis.Decisions, types.Decision{
				Action:            types.ActionEnter,
				Symbol:            symbol,
				At:                forecast.At,
				Utility:           utility,
				Opportunity:       recommendedAction == ActionEnter,
				AllocationHaircut: 0.1,
				AllocationClass:   "normal",
				Alternatives: map[string]float64{
					"enter":   utility,
					"nothing": 0,
				},
				ExpectedFees:      forecast.ExpectedFees,
				ExpectedSpread:    forecast.ExpectedSpread,
				ExpectedReturn:    forecast.ExpectedReturn,
				ExpectedImpact:    forecast.ExpectedImpact,
				AdverseSelection:  forecast.ExpectedAdverseSelection,
				Uncertainty:       forecast.Uncertainty,
				Confidence:        forecast.Confidence,
				OpportunityMargin: utility,
				CognitiveLead:     cognition.LookaheadScore,
				BasinConfidence:   cognition.Confidence,
				ReferencePrice:    forecast.ReferencePrice.Copy(),
				ValidThroughEpoch: forecast.ExpiresEpoch,
				ForecastSource:    forecast.Source,
				ForecastModel:     forecast.ModelVersion,
				ForecastEpoch:     forecast.SourceEpoch,
				CalibrationCount:  forecast.CalibrationSamples,
				Cause:             "causal_mcts_entry",
				Reason:            "causal MCTS search recommended entry trajectory",
			})
			continue
		}

		thesis.Decisions = append(thesis.Decisions, evaluator.reject(
			forecast, utility, "mcts_rejected",
			"causal MCTS trajectory search did not select entry action",
		))
	}
}

func getCausalHistoryRows(thesis *types.Thesis, symbol string) [][]float64 {
	if thesis == nil || thesis.Causal == nil {
		return nil
	}
	val, ok := thesis.Causal.Load(symbol)
	if !ok || val == nil {
		return nil
	}
	m, ok := val.(map[string]any)
	if !ok {
		return nil
	}

	// Reconstruct history rows if stored
	if rowsRaw, ok := m["historyRows"].([][]float64); ok {
		return rowsRaw
	}

	// Single row fallback
	row := []float64{
		getFloat(m, "energy"),
		getFloat(m, "surprise"),
		getFloat(m, "intervention"),
		getFloat(m, "doExpectation"),
	}
	return [][]float64{row}
}

func getFloat(m map[string]any, key string) float64 {
	if v, ok := m[key].(float64); ok {
		return v
	}
	return 0.0
}

// ManageContinuation scores continuation (Hold) for active open holdings.
func (evaluator Evaluator) ManageContinuation(
	thesis *types.Thesis,
	desk *broker.Desk,
	price *broker.Price,
) {
	if thesis == nil || desk == nil || price == nil {
		return
	}

	for position := range desk.Positions() {
		if position.Status != types.OPEN {
			continue
		}

		if isExiting(thesis, position.Holding.Symbol) {
			continue
		}

		forecast, found := selectForecast(thesis.Forecasts, position.Holding.Symbol)

		if !found || !forecast.Eligible() {
			thesis.Decisions = append(thesis.Decisions, types.Decision{
				Action:           types.ActionHold,
				Symbol:           position.Holding.Symbol,
				Cause:            "continuation",
				Reason:           "awaiting eligible forecast for continuation scoring",
				ProposedQuantity: decimal.NewFromInt64(0),
				ProposedNotional: decimal.NewFromInt64(0),
				Alternatives:     map[string]float64{"hold": 0},
			})

			continue
		}

		fee, err := evaluator.price.Fee(position.Holding.Symbol)

		if err != nil {
			thesis.Decisions = append(thesis.Decisions, types.Decision{
				Action:           types.ActionHold,
				Symbol:           position.Holding.Symbol,
				Cause:            "continuation",
				Reason:           "fee rate unavailable for continuation scoring",
				ProposedQuantity: decimal.NewFromInt64(0),
				ProposedNotional: decimal.NewFromInt64(0),
				Alternatives:     map[string]float64{"hold": 0},
			})

			errnie.Error(errnie.Err(
				errnie.NotFound,
				"continuation: fee rate unavailable for "+position.Holding.Symbol,
				err,
			))

			continue
		}

		holdUtility := forecast.ExpectedReturn.Float64() - forecast.Uncertainty

		exitCost := fee.Add(
			forecast.ExpectedSpread.Div(decimal.NewFromInt64(2)),
		).Add(forecast.ExpectedImpact)

		doExpectation, uplift, noise, causalReady := getCausalMetrics(thesis, forecast.Symbol)
		cognition := getCognition(thesis, forecast.Symbol)

		if causalReady {
			holdUtility += doExpectation + uplift - noise
		}

		if cognition.Ready {
			holdUtility += cognition.LookaheadScore * cognition.Confidence
		}

		// Apply Graph Contradiction Penalty to Hold Utility
		_, contradicts, hasGraph := inspectGraph(thesis, forecast.Symbol)

		if hasGraph && contradicts > 0 {
			holdUtility -= (contradicts * 0.1)
		}

		thesis.Decisions = append(thesis.Decisions, types.Decision{
			Action:  types.ActionHold,
			Symbol:  forecast.Symbol,
			At:      forecast.At,
			Utility: holdUtility,
			Alternatives: map[string]float64{
				"hold": holdUtility,
				"exit": -exitCost.Float64(),
			},
			ProposedNotional:  decimal.NewFromInt64(0),
			ProposedQuantity:  decimal.NewFromInt64(0),
			ExpectedReturn:    forecast.ExpectedReturn,
			ExpectedFees:      fee,
			ExpectedSpread:    forecast.ExpectedSpread.Div(decimal.NewFromInt64(2)),
			ExpectedImpact:    forecast.ExpectedImpact,
			AdverseSelection:  forecast.ExpectedAdverseSelection,
			Uncertainty:       forecast.Uncertainty,
			Confidence:        forecast.Confidence,
			OpportunityMargin: holdUtility + exitCost.Float64(),
			CognitiveLead:     cognition.LookaheadScore,
			BasinConfidence:   cognition.Confidence,
			ReferencePrice:    forecast.ReferencePrice.Copy(),
			ValidThroughEpoch: forecast.ExpiresEpoch,
			ForecastSource:    forecast.Source,
			ForecastEpoch:     forecast.SourceEpoch,
			Cause:             "continuation",
			Reason:            "continuation holds active position",
		})
	}
}

func (evaluator Evaluator) stampFriction(
	forecast *types.Forecasts,
) {
	fee, err := evaluator.price.Fee(forecast.Symbol)

	if err != nil {
		forecast.ExpectedFees = decimal.NewFromInt64(0)
		forecast.ExpectedSpread = decimal.NewFromInt64(0)
		forecast.ExpectedImpact = decimal.NewFromInt64(0)
		forecast.FrictionReady = false

		errnie.Error(errnie.Err(
			errnie.NotFound,
			"friction: fee rate unavailable for "+forecast.Symbol,
			err,
		))

		return
	}

	forecast.ExpectedFees = fee

	if forecast.ExpectedSpread == nil {
		forecast.ExpectedSpread = decimal.NewFromInt64(0)
	}

	forecast.ExpectedImpact = forecast.ExpectedSpread.Mul(
		decimal.NewFromFloat64(0.1),
	)
	forecast.FrictionReady = true
}

func (e Evaluator) reject(
	forecast types.Forecasts,
	utility float64,
	cause, reason string,
) types.Decision {
	return types.Decision{
		Action:            types.ActionNothing,
		Symbol:            forecast.Symbol,
		At:                forecast.At,
		Utility:           utility,
		Alternatives:      map[string]float64{"enter": utility, "nothing": 0},
		ReferencePrice:    forecast.ReferencePrice.Copy(),
		ValidThroughEpoch: forecast.ExpiresEpoch,
		ForecastSource:    forecast.Source,
		ForecastModel:     forecast.ModelVersion,
		ForecastEpoch:     forecast.SourceEpoch,
		CalibrationCount:  forecast.CalibrationSamples,
		ExpectedReturn:    forecast.ExpectedReturn,
		ExpectedFees:      forecast.ExpectedFees,
		ExpectedSpread:    forecast.ExpectedSpread,
		ExpectedImpact:    forecast.ExpectedImpact,
		AdverseSelection:  forecast.ExpectedAdverseSelection,
		Uncertainty:       forecast.Uncertainty,
		Confidence:        forecast.Confidence,
		OpportunityMargin: utility,
		Cause:             cause,
		Reason:            reason,
	}
}

// Helper Functions

func selectForecasts(rows []types.Forecasts) map[string]types.Forecasts {
	selected := make(map[string]types.Forecasts, len(rows))
	for _, forecast := range rows {
		prior, found := selected[forecast.Symbol]
		if !found || forecast.SourceEpoch > prior.SourceEpoch {
			selected[forecast.Symbol] = forecast
			continue
		}
		if forecast.SourceEpoch == prior.SourceEpoch && forecast.Eligible() && !prior.Eligible() {
			selected[forecast.Symbol] = forecast
		}
	}
	return selected
}

func selectForecast(rows []types.Forecasts, symbol string) (types.Forecasts, bool) {
	selected := selectForecasts(rows)
	forecast, found := selected[symbol]
	return forecast, found
}

func getCausalMetrics(thesis *types.Thesis, symbol string) (doExp, uplift, noise float64, ready bool) {
	if thesis == nil || thesis.Causal == nil {
		return 0, 0, 0, false
	}

	val, ok := thesis.Causal.Load(symbol)

	if !ok || val == nil {
		return 0, 0, 0, false
	}

	m, ok := val.(map[string]any)

	if !ok {
		return 0, 0, 0, false
	}

	getFloat := func(key string) float64 {
		if v, ok := m[key].(float64); ok {
			return v
		}
		return 0.0
	}

	doExp = getFloat("doExpectation")
	uplift = getFloat("uplift")
	noise = getFloat("noise")
	strength := getFloat("strength")
	confidence := getFloat("confidence")

	ready = (strength > 0 || confidence > 0) && (uplift != 0 || doExp != 0)
	return doExp, uplift, noise, ready
}

func getCognition(thesis *types.Thesis, symbol string) types.Cognition {
	val, ok := thesis.Cognition.Load(symbol)

	if !ok || val == nil {
		return types.Cognition{}
	}

	if cog, ok := val.(types.Cognition); ok {
		return cog
	}

	return types.Cognition{}
}

func inspectGraph(thesis *types.Thesis, symbol string) (supports, contradicts float64, hasGraph bool) {
	val, ok := thesis.Graphs.Load("market_graph")

	if !ok || val == nil {
		return 0, 0, false
	}

	g, ok := val.(*graph.Graph)

	if !ok || g == nil {
		return 0, 0, false
	}

	for _, edge := range g.Edges {
		fromNode, fromOk := g.Nodes[edge.From]
		toNode, toOk := g.Nodes[edge.To]

		if (fromOk && fromNode.Symbol == symbol) || (toOk && toNode.Symbol == symbol) {
			switch edge.Relation {
			case graph.RelationSupports,
				graph.RelationConditions,
				graph.RelationLeads:
				supports += edge.Weight * edge.Confidence
			case graph.RelationContradicts,
				graph.RelationStaleRelativeTo,
				graph.RelationIncomparableWith:
				contradicts += edge.Weight * edge.Confidence
			}
		}
	}

	return supports, contradicts, true
}

func getOccupiedSymbols(thesis *types.Thesis, desk *broker.Desk) map[string]struct{} {
	occupied := make(map[string]struct{})

	if thesis == nil || desk == nil {
		return occupied
	}

	for position := range desk.Positions() {
		if position.Status != types.CLOSED {
			occupied[position.Holding.Symbol] = struct{}{}
		}
	}

	thesis.Lifecycle.Range(func(key, value any) bool {
		if symbol, ok := key.(string); ok {
			if state, isStr := value.(string); isStr {
				switch state {
				case types.LifecycleEntrySelected,
					types.LifecycleEntrySubmitted,
					types.LifecyclePartiallyEntered,
					types.LifecycleManaging:
					occupied[symbol] = struct{}{}
				}
			}
		}

		return true
	})

	return occupied
}

func isExiting(thesis *types.Thesis, symbol string) bool {
	if val, ok := thesis.Lifecycle.Load(symbol); ok {
		if state, isStr := val.(string); isStr {
			return state == types.LifecycleExitSelected || state == types.LifecycleExitSubmitted
		}
	}

	return false
}
