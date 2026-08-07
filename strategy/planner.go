package strategy

import (
	"context"
	"fmt"
	"math"
	"sync"

	"github.com/spf13/viper"
	"github.com/theapemachine/api-go/v2/pkg/decimal"
	"github.com/theapemachine/datura"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/nomagique/mcts"
	"github.com/theapemachine/symm/audit"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/logic"
	"github.com/theapemachine/symm/types"
	"github.com/theapemachine/symm/utils"
)

type Planner struct {
	ctx               context.Context
	cancel            context.CancelFunc
	status            types.Status
	ui                chan []byte
	subscriptions     map[string]*types.Subscription[any]
	subscribers       *sync.Map
	recorder          *audit.Recorder
	mctsEngine        *mcts.CausalMCTS
	minimumConfidence float64
	maxFraction       float64
	desk              *broker.Desk
}

func NewPlanner(
	ctx context.Context,
	uiHub chan []byte,
	analyzer *logic.Analyzer,
	recorder *audit.Recorder,
	desk *broker.Desk,
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
				"analyzer", types.NewSubscription[any](),
			),
		},
		subscribers: &sync.Map{},
		recorder:    recorder,
		mctsEngine:  mctsEngine,
		minimumConfidence: viper.GetFloat64(
			"trading.resonance.minimum_confidence",
		),
		maxFraction: viper.GetFloat64("trading.allocation.max_fraction"),
		desk:        desk,
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
					errnie.Error(errnie.Err(
						errnie.UnprocessableContent,
						"planner: invalid thesis",
						nil,
					))

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
	if thesis.LogicAnalyzed() {
		decisions := planner.decisions(thesis)

		if len(decisions) != 0 {
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

			utils.Publish(planner.ui, datura.NewMap("strategy", datura.NewMap(
				"evaluated", true,
				"outcome", "decisions",
				"decisions", decisions,
			)))
		}
	}

	utils.Fanout(planner.subscribers, "planner", thesis)
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
				"planner: causal search failed: "+err.Error(),
				err,
			))

			return true
		}

		decision = planner.admit(thesis, decision)
		decision, err = planner.size(decision)

		if err != nil {
			errnie.Error(errnie.Err(
				errnie.UnprocessableContent,
				"planner: entry sizing failed: "+err.Error(),
				err,
			))

			decision = types.NewDecision(types.ActionNothing, symbol)
			decision.Reason = err.Error()
		}

		decision.At = thesis.At
		thesis.Decisions.Store(symbol, decision)
		decisions = append(decisions, *decision)

		return true
	})

	return decisions
}

/*
size completes the two execution fields an admitted Enter requires. Quantity
is the configured cash fraction normalized by the venue price surface. The
Stoploss is built from the same forecast Planner admitted, so its initial floor
is the lowest predicted tick in that confidence-supported horizon.
*/
func (planner *Planner) size(
	decision *types.Decision,
) (*types.Decision, error) {
	if decision == nil || decision.Action != types.ActionEnter {
		return decision, nil
	}

	if planner.desk == nil || planner.maxFraction <= 0 || planner.maxFraction > 1 {
		return decision, fmt.Errorf("planner: executable desk and allocation required")
	}

	cash := planner.desk.Balance().Cash()

	if cash == nil || cash.Sign() <= 0 {
		return decision, fmt.Errorf("planner: positive quote cash required")
	}

	notional := decimal.ExactMul(
		cash,
		decimal.NewFromFloat64(planner.maxFraction),
	)
	price := planner.desk.Price()
	tick := price.Tick(decision.Symbol)

	if tick == nil || tick.Ask == nil || tick.Ask.Sign() <= 0 {
		return decision, fmt.Errorf("planner: executable ask required")
	}

	quantity := price.Quantity(decision.Symbol, notional)

	if quantity == nil || quantity.Sign() <= 0 {
		return decision, fmt.Errorf("planner: allocation produced no executable quantity")
	}

	pair := planner.desk.Instrument().Pair(decision.Symbol)

	if pair.Symbol == "" || pair.TickSize.Sign() <= 0 {
		return decision, fmt.Errorf("planner: instrument tick size required")
	}

	fee := price.Fee(decision.Symbol)

	if fee == nil || fee.Fee == nil || fee.Fee.Sign() < 0 {
		return decision, fmt.Errorf("planner: taker fee required")
	}

	feeRate := decimal.ExactDiv(fee.Fee, decimal.NewFromInt64(100))
	stoploss, err := types.NewStoploss(
		planner.ctx,
		decision.Symbol,
		tick.Ask,
		decision.Forecast,
		&pair.TickSize,
		feeRate,
		feeRate,
	)

	if err != nil {
		return decision, fmt.Errorf("planner: strategy stoploss: %w", err)
	}

	decision.AvailableCapital = cash
	decision.ProposedNotional = notional
	decision.ProposedQuantity = quantity
	decision.ReferencePrice = tick.Ask
	decision.Mark = tick.Bid
	decision.Stoploss = stoploss

	return decision, nil
}

/*
admit applies the provisional Resonance admission policy to an Enter verdict.
The forecast remains on the decision because the position's Stoploss uses its
lowest supported path point as the pre-profit-lock floor.
*/
func (planner *Planner) admit(
	thesis *types.Thesis,
	decision *types.Decision,
) *types.Decision {
	if decision == nil || decision.Action != types.ActionEnter {
		return decision
	}

	readingRaw, found := thesis.Resonance.Load(decision.Symbol)

	if !found {
		return types.NewDecision(types.ActionNothing, decision.Symbol)
	}

	reading, valid := readingRaw.(types.ResonanceReading)

	if !valid || reading.Forecast == nil ||
		reading.Forecast.Confidence < planner.minimumConfidence ||
		reading.Forecast.Validate() != nil {
		return types.NewDecision(types.ActionNothing, decision.Symbol)
	}

	decision.Forecast = reading.Forecast
	decision.Confidence = reading.Forecast.Confidence

	return decision
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

	if readyOK && !ready {
		return types.NewDecision(types.ActionNothing, symbol), nil
	}

	rows, rowsOK := causal["historyRows"].([][]float64)
	treatment, treatmentOK := causal["treatmentLevel"].(float64)

	if !readyOK || !ready || !rowsOK ||
		len(rows) < planner.mctsEngine.MinRows || !treatmentOK ||
		math.IsNaN(treatment) || math.IsInf(treatment, 0) {
		return types.NewDecision(types.ActionNothing, symbol), nil
	}

	latest := rows[len(rows)-1]

	if len(latest) != 4 {
		return types.NewDecision(types.ActionNothing, symbol), nil
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
