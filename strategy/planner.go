package strategy

import (
	"context"
	"math"
	"sync"

	"github.com/theapemachine/datura"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/nomagique/mcts"
	"github.com/theapemachine/symm/audit"
	"github.com/theapemachine/symm/logic"
	"github.com/theapemachine/symm/types"
	"github.com/theapemachine/symm/utils"
)

type Planner struct {
	ctx              context.Context
	cancel           context.CancelFunc
	status           types.Status
	ui               chan []byte
	subscriptions    map[string]*types.Subscription[any]
	subscribers      *sync.Map
	recorder         *audit.Recorder
	mctsEngine       *mcts.CausalMCTS
}

func NewPlanner(
	ctx context.Context,
	uiHub chan []byte,
	analyzer *logic.Analyzer,
	recorder *audit.Recorder,
) *Planner {
	ctx, cancel := context.WithCancel(ctx)
	mctsEngine := mcts.NewCausalMCTS(
		NewCausalEngineAdapter(),
		math.Sqrt2,
		1,
		mctsMinimumCausalRows,
		2,
		3,
		[]int{0, 1},
		[]int{0, 1, 2},
		false,
	)

	planner := &Planner{
		ctx:    ctx,
		cancel: cancel,
		status: types.READY,
		ui:     uiHub,
		subscriptions: map[string]*types.Subscription[any]{
			"analyzer": analyzer.Subscribe(
				"analyzer", types.NewLatestSubscription[any](),
			),
		},
		subscribers: &sync.Map{},
		recorder:    recorder,
		mctsEngine:  mctsEngine,
	}

	planner.run()
	return planner
}

func (planner *Planner) Status() types.Status {
	return planner.status
}

func (planner *Planner) Subscribe(
	key string, subscription *types.Subscription[any],
) *types.Subscription[any] {
	subscribers, ok := planner.subscribers.LoadOrStore(
		key, []*types.Subscription[any]{subscription},
	)

	if ok {
		planner.subscribers.Store(key, append(
			subscribers.([]*types.Subscription[any]),
			subscription,
		))
	}

	return subscription
}

func (planner *Planner) run() {
	go func() {
		for {
			select {
			case <-planner.ctx.Done():
				return
			case in := <-planner.subscriptions["analyzer"].Channel:
				thesis, ok := in.(*types.Thesis)

				if !ok {
					continue
				}

				planner.Update(thesis)
			}
		}
	}()
}

func (planner *Planner) Close() error {
	planner.cancel()
	return nil
}

func (planner *Planner) Update(thesis *types.Thesis) {
	if thesis == nil || !thesis.LogicAnalyzed() {
		return
	}

	decisions := planner.decisions(thesis)

	if len(decisions) == 0 {
		return
	}

	planner.subscribers.Range(func(key, value any) bool {
		name, ok := key.(string)

		if !ok || name == "planner" {
			return true
		}

		for _, subscriber := range value.([]*types.Subscription[any]) {
			subscriber.SendLatest(decisions)
		}

		return true
	})

	if planner.recorder != nil {
		if err := planner.recorder.Write(decisions); err != nil {
			errnie.Error(errnie.Err(
				errnie.IO,
				"planner: decision audit failed",
				err,
			))
		}
	}

	thesis.Stamp(types.SourcePlanner)
	utils.Fanout(planner.subscribers, "planner", thesis)

	utils.Publish(planner.ui, datura.NewMap("strategy", datura.NewMap(
		"evaluated", true,
		"outcome", "decisions",
		"decisions", decisions,
	)))
}

/*
decisions asks causal MCTS for one binary verdict per ready causal artifact.
*/
func (planner *Planner) decisions(thesis *types.Thesis) []types.Decision {
	decisions := make([]types.Decision, 0)

	thesis.Causal.Range(func(key, value any) bool {
		symbol, symbolOK := key.(string)
		causal, causalOK := value.(map[string]any)

		if !symbolOK || symbol == "" || !causalOK {
			errnie.Error(errnie.Err(
				errnie.UnprocessableContent,
				"planner: invalid causal artifact",
				nil,
			))

			return true
		}

		decision, err := planner.search(symbol, causal)

		if err != nil {
			errnie.Error(errnie.Err(
				errnie.UnprocessableContent,
				"planner: causal search failed",
				err,
			))

			return true
		}

		decision.At = thesis.At
		thesis.Decisions.Store(symbol, decision)
		decisions = append(decisions, *decision)

		return true
	})

	return decisions
}

/*
search compares the observed treatment with the standing-aside do(0)
intervention using the existing causal MCTS.
*/
func (planner *Planner) search(
	symbol string,
	causal map[string]any,
) (*types.Decision, error) {
	ready, readyOK := causal["ready"].(bool)
	rows, rowsOK := causal["historyRows"].([][]float64)
	treatment, treatmentOK := causal["treatmentLevel"].(float64)

	if !readyOK || !ready || !rowsOK ||
		len(rows) < planner.mctsEngine.MinRows || !treatmentOK ||
		math.IsNaN(treatment) || math.IsInf(treatment, 0) {
		return nil, errnie.Err(
			errnie.Validation,
			"planner: complete causal rows and treatment required",
			nil,
		)
	}

	latest := rows[len(rows)-1]

	if len(latest) != 4 {
		return nil, errnie.Err(
			errnie.Validation,
			"planner: causal row must contain two controls, treatment, and target",
			nil,
		)
	}

	root := StrategyState{
		Symbol:    symbol,
		Condition: latest[0],
		Contagion: latest[1],
		Treatment: treatment,
	}
	action, err := planner.mctsEngine.Search(
		root,
		mctsSearchIterations,
		rows,
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

	return types.NewDecision(decisionAction, symbol), nil
}
