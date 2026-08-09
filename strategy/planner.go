package strategy

import (
	"context"
	"fmt"
	"math"

	"github.com/spf13/viper"
	"github.com/theapemachine/datura"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/nomagique/mcts"
	"github.com/theapemachine/symm/audit"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/types"
	"github.com/theapemachine/symm/utils"
)

type Planner struct {
	ctx               context.Context
	cancel            context.CancelFunc
	status            types.Status
	ui                chan []byte
	thesis            *types.Thesis
	semaphore         chan struct{}
	recorder          *audit.Recorder
	mctsEngine        *mcts.CausalMCTS
	minimumConfidence float64
	maxFraction       float64
	desk              *broker.Desk
}

func NewPlanner(
	ctx context.Context,
	uiHub chan []byte,
	thesis *types.Thesis,
	recorder *audit.Recorder,
	desk *broker.Desk,
) *Planner {
	ctx, cancel := context.WithCancel(ctx)
	mctsEngine := mcts.NewCausalMCTS(
		NewCausalEngineAdapter(),
		math.Sqrt2,
		1,
		0,
		2,
		3,
		[]int{0, 1},
		[]int{0, 1, 2},
		false,
	)

	planner := &Planner{
		ctx:        ctx,
		cancel:     cancel,
		status:     types.READY,
		ui:         uiHub,
		thesis:     thesis,
		semaphore:  make(chan struct{}, 1),
		recorder:   recorder,
		mctsEngine: mctsEngine,
		minimumConfidence: viper.GetFloat64(
			"trading.resonance.minimum_confidence",
		),
		maxFraction: viper.GetFloat64("trading.allocation.max_fraction"),
		desk:        desk,
	}

	planner.thesis.Subscribe(types.SourcePlanner, planner.semaphore)
	planner.run()
	return planner
}

func (planner *Planner) Status() types.Status {
	return planner.status
}

func (planner *Planner) run() {
	go func() {
		for {
			select {
			case <-planner.ctx.Done():
				return
			case <-planner.semaphore:
				planner.Update(planner.thesis)
			}
		}
	}()
}

func (planner *Planner) Close() error {
	planner.cancel()
	return nil
}

func (planner *Planner) Update(thesis *types.Thesis) {
	if !thesis.LogicAnalyzed() || thesis.StrategyDecided() {
		return
	}

	decisions, err := planner.decisions(thesis)

	if err != nil {
		errnie.Error(errnie.Err(
			errnie.UnprocessableContent,
			"planner: decision pass failed: "+err.Error(),
			err,
		))

		return
	}

	if len(decisions) != 0 && planner.recorder != nil {
		if err := planner.recorder.Write(decisions); err != nil {
			errnie.Error(errnie.Err(
				errnie.IO,
				"planner: decision audit failed",
				err,
			))
		}
	}

	thesis.Stamp(types.SourcePlanner)

	if len(decisions) != 0 {
		utils.Publish(planner.ui, datura.NewMap("strategy", datura.NewMap(
			"evaluated", true,
			"outcome", "decisions",
			"decisions", decisions,
		)))
	}

	thesis.Fanout(types.SourcePlanner)
}

/* decisions evaluates one binary verdict per ready causal artifact. */
func (planner *Planner) decisions(thesis *types.Thesis) ([]types.Decision, error) {
	decisions := make([]types.Decision, 0)
	var decisionsErr error

	thesis.Causal.Range(func(key, value any) bool {
		symbol, symbolOK := key.(string)
		causal, causalOK := value.(map[string]any)

		if !symbolOK || symbol == "" || !causalOK {
			decisionsErr = fmt.Errorf("planner: invalid causal artifact")
			return false
		}

		decision, err := planner.search(thesis, symbol, causal)

		if err != nil {
			decisionsErr = fmt.Errorf(
				"planner: causal search failed for %s: %w",
				symbol,
				err,
			)

			return false
		}

		decision, err = planner.size(decision)

		if err != nil {
			errnie.Error(errnie.Err(
				errnie.UnprocessableContent,
				"planner: entry sizing failed: "+err.Error(),
				err,
			))

			decision.Action = types.ActionNothing
			decision.Reason = err.Error()
		}

		decision.At = thesis.At
		thesis.Decisions.Store(symbol, decision)
		decisions = append(decisions, *decision)

		return true
	})

	return decisions, decisionsErr
}

/*
admit gates on predictive confidence and returns graph evidence separately.
*/
func (planner *Planner) admit(
	thesis *types.Thesis,
	decision *types.Decision,
) (*types.Decision, graphEvidence, error) {
	evidence := graphEvidence{}

	if decision == nil || decision.Action != types.ActionEnter {
		return decision, evidence, nil
	}

	forecast, evidence, err := forecastWithGraphEvidence(
		thesis,
		decision.Symbol,
	)

	if err != nil {
		return nil, evidence, err
	}

	if !forecast.ConfidenceReady {
		rejected := types.NewDecision(types.ActionNothing, decision.Symbol)
		rejected.Forecast = forecast
		rejected.Confidence = forecast.Confidence
		rejected.Reason = "planner: forecast predictive distribution is not ready"

		return rejected, evidence, nil
	}

	if forecast.Confidence < planner.minimumConfidence {
		rejected := types.NewDecision(types.ActionNothing, decision.Symbol)
		rejected.Forecast = forecast
		rejected.Confidence = forecast.Confidence
		rejected.Reason = fmt.Sprintf(
			"planner: forecast confidence %.6f is below configured minimum %.6f",
			forecast.Confidence,
			planner.minimumConfidence,
		)

		return rejected, evidence, nil
	}

	drawdown, err := forecast.WorstIntermediateDrawdown()

	if err != nil {
		return nil, evidence, err
	}

	decision.Forecast = forecast
	decision.Confidence = forecast.Confidence
	decision.Uncertainty = drawdown
	decision.AllocationClass = allocationClassNormal
	decision.Opportunity = false

	return decision, evidence, nil
}

/* search compares entry with the standing-aside do(0) intervention. */
func (planner *Planner) search(
	thesis *types.Thesis,
	symbol string,
	causal map[string]any,
) (*types.Decision, error) {
	rows, rowsOK := causal["historyRows"].([][]float64)
	treatment, treatmentOK := causal["treatmentLevel"].(float64)
	precision, precisionOK := causal["precision"].(float64)
	samples, samplesOK := causal["samples"].(int)

	if !rowsOK || len(rows) == 0 || !treatmentOK || !precisionOK ||
		!samplesOK || samples != len(rows) ||
		math.IsNaN(treatment) || math.IsInf(treatment, 0) ||
		math.IsNaN(precision) || math.IsInf(precision, 0) ||
		precision < 0 || precision > 1 {
		return nil, errnie.Err(
			errnie.Validation,
			"planner: complete finite causal estimate required",
			nil,
		)
	}

	latest := rows[len(rows)-1]

	if len(latest) != 4 {
		return nil, errnie.Err(
			errnie.Validation,
			"planner: causal row shape is invalid",
			nil,
		)
	}

	candidate, evidence, err := planner.admit(
		thesis,
		types.NewDecision(types.ActionEnter, symbol),
	)

	if err != nil {
		return nil, err
	}

	graphReward, err := evidence.Reward(rows, planner.mctsEngine.TargetCol)

	if err != nil {
		return nil, err
	}

	rootState := StrategyState{
		Symbol:      symbol,
		Condition:   latest[0],
		Contagion:   latest[1],
		Treatment:   treatment,
		CanEnter:    candidate.Action == types.ActionEnter,
		GraphReward: graphReward,
		Precision:   precision,
	}
	searchIterations := len(rows)
	searchRoot, action, err := planner.mctsEngine.Search(
		rootState, searchIterations, rows,
	)

	if err != nil {
		return nil, err
	}

	decisionAction := strategyAction(action)

	if decisionAction == "" {
		return nil, errnie.Err(
			errnie.Validation,
			"planner: causal search returned an unsupported action",
			nil,
		)
	}

	candidate.CausalPrecision = precision
	candidate.Trace = &types.DecisionTrace{
		MCTS: types.DecisionMCTSTrace{
			Treatment:         treatment,
			Precision:         precision,
			Iterations:        searchIterations,
			Branches:          mctsBranches(searchRoot),
			RecommendedAction: decisionAction,
		},
	}

	if decisionAction == types.ActionEnter {
		return candidate, nil
	}

	candidate.Action = decisionAction

	if candidate.Reason != "" {
		return candidate, nil
	}

	candidate.Reason = "planner: causal search selected standing aside"
	return candidate, nil
}
