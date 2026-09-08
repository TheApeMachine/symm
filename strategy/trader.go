package strategy

import (
	"context"
	"fmt"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/book"
	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/broker/position"
	"github.com/theapemachine/symm/hindsight/recording"
	"github.com/theapemachine/symm/kraken/websocket"
	"github.com/theapemachine/symm/nomagique/learning/associative/agent"
	"github.com/theapemachine/symm/nomagique/learning/associative/reward"
)

/*
	Action identifies a resource fraction, independently of the current price.

Power is a binary subdivision of available cash or held inventory.
*/
type Action struct {
	Kind   string
	Power  uint16
	Reduce bool
}

/*
	Trader adapts an independent account and its normal position regulators to

associative learning. It is owned by Learner's serialized state transitions.
*/
type Trader struct {
	Balance                                       *broker.Balance
	Positions                                     map[string]*position.Regulator
	Execution                                     Execution
	Initial, Equity, Profit, Realized, Unrealized *decimal.Decimal
	Wealth                                        float64
	At                                            time.Time
	Version, Fills                                uint64
	Status                                        string
	Alternatives                                  []Action
	Quantities                                    map[Action]*decimal.Decimal
	Evaluations                                   map[uint64]*Evaluation
	LastEvaluation                                *Evaluation
	Recorder                                      *recording.Session
	ID                                            int
	Fees                                          *decimal.Decimal
	ctx                                           context.Context
	api                                           *websocket.API
}

func NewTrader(
	ctx context.Context,
	api *websocket.API,
	price *broker.Price,
	quote string,
	cash *decimal.Decimal,
) (*Trader, error) {
	balance, err := broker.NewFundedBalance(quote, cash)

	if err != nil {
		return nil, errnie.Error(errnie.Err(
			errnie.Validation,
			"[trader] failed to get balance",
			err,
		))
	}

	return &Trader{
		ctx:     ctx,
		api:     api,
		Balance: balance,
		Execution: Execution{
			Price:   price,
			Balance: balance,
		},
		Initial:     cash,
		Equity:      cash,
		Realized:    decimal.NewFromInt64(0),
		Unrealized:  decimal.NewFromInt64(0),
		Fees:        decimal.NewFromInt64(0),
		Profit:      decimal.NewFromInt64(0),
		Positions:   make(map[string]*position.Regulator),
		Evaluations: make(map[uint64]*Evaluation),
	}, nil
}

/*
	Feasible filters only available resources, venue minima and executable depth.

Book absence permits waiting; it is reported rather than invented as a price.
*/
func (trader *Trader) Feasible(symbol string) ([]Action, []uint64, error) {
	idle := Action{Kind: "wait"}
	held := decimal.NewFromInt64(0)

	if regulator := trader.Positions[symbol]; regulator != nil {
		held = regulator.Holding.Qty
	}

	if held.Sign() > 0 {
		idle.Kind = "hold"
	}
	trader.Alternatives = []Action{idle}
	trader.Quantities = make(map[Action]*decimal.Decimal)
	trader.Status = "book unavailable"
	var bid, ask *decimal.Decimal
	trader.api.Book(symbol, func(book *book.Book) {
		if book != nil && book.BestBid() != nil && book.BestAsk() != nil {
			bid, ask = book.BestBid().Price, book.BestAsk().Price
		}
	})
	state := []uint64{uint64(1) << 63}

	if held.Sign() > 0 {
		state[0]++
	}

	if bid == nil || ask == nil || trader.Execution.Price.FeeIfAvailable(symbol) == nil {
		return trader.Alternatives, state, nil
	}
	trader.Status = "learning"

	if held.Sign() > 0 {
		if err := trader.sizes(symbol, Action{Kind: "exit", Reduce: true}, held, bid); err != nil {
			return nil, nil, err
		}
	}

	if trader.Balance.Cash().Sign() > 0 {
		quantity, err := trader.Execution.Price.Affordable(symbol, trader.Balance.Cash(), ask)

		if err != nil {
			return nil, nil, err
		}
		kind := "enter"

		if held.Sign() > 0 {
			kind = "scale"
		}

		if err := trader.sizes(symbol, Action{Kind: kind}, quantity, ask); err != nil {
			return nil, nil, err
		}
	}
	return trader.Alternatives, state, nil
}

/*
	sizes enumerates the binary resource fractions down to the venue's minimum.

No fixed exposure cap or prediction threshold participates.
*/
func (trader *Trader) sizes(symbol string, action Action, quantity, unit *decimal.Decimal) error {
	for trader.Execution.Price.Tradable(symbol, quantity, unit) {
		usable := false

		if action.Reduce {
			surface, err := trader.Execution.Price.Surface(symbol, quantity, trader.At)
			usable = err == nil && surface.FullyExecutable

			if err != nil {
				trader.Status = err.Error()

				if !errnie.IsUnprocessableContent(err) {
					return err
				}
			}
		}

		if !action.Reduce {
			cost, err := trader.Execution.Price.EntryCost(symbol, quantity)
			usable = err == nil && cost.Total.Cmp(trader.Balance.Cash()) <= 0

			if err != nil {
				trader.Status = err.Error()

				if !errnie.IsUnprocessableContent(err) {
					return err
				}
			}
		}

		if usable {
			trader.Alternatives = append(trader.Alternatives, action)
			trader.Quantities[action] = quantity
		}
		action.Power++

		if action.Reduce {
			action.Kind = "scale"
		}
		next, err := trader.api.Normalizer().FormatSize(symbol, quantity.Div(decimal.NewFromInt64(2)))

		if err != nil {
			return errnie.Error(err)
		}

		if next.Cmp(quantity) >= 0 {
			break
		}
		quantity = next
	}
	return nil
}

/*
	Execute submits through the normal regulator and applies its actual fill.

The retained evaluation points to the original decision, never a second model.
*/
func (trader *Trader) Execute(decision *agent.Decision[Action]) error {
	evaluation := &Evaluation{Decision: decision, Trader: trader.ID, Initial: trader.Initial}
	trader.Evaluations[decision.ID] = evaluation
	regulator := trader.Positions[decision.Label]
	evaluation.Quantity = decimal.NewFromInt64(0)

	if regulator != nil {
		evaluation.Quantity = regulator.Holding.Qty
	}
	tick := trader.Execution.Price.Tick(decision.Label)

	if tick != nil {
		evaluation.Reference = tick.Last
	}

	if decision.Action.Kind == "wait" || decision.Action.Kind == "hold" {
		for _, action := range trader.Alternatives {
			if action.Kind == "enter" && evaluation.Opportunity == nil {
				evaluation.Opportunity = trader.Quantities[action]
			}
		}
		return trader.Recorder.WriteLearning(evaluation)
	}

	if regulator == nil {
		regulator = position.NewRegulator(trader.ctx, trader.api, trader.Execution.Price, decision.Label, nil)
		regulator.Orders = &trader.Execution
		trader.Positions[decision.Label] = regulator
	}
	side := broker.BUY

	if decision.Action.Reduce {
		side = broker.SELL
	}
	trader.Execution.At = decision.At

	if err := regulator.Submit(fmt.Sprint(decision.ID), side, trader.Quantities[decision.Action]); err != nil {
		return err
	}

	if err := regulator.Apply(trader.Execution.Fill); err != nil {
		return err
	}
	evaluation.Quantity = trader.Execution.Fill.CumQty
	evaluation.Cost = trader.Execution.Fill.CumCost
	evaluation.Fee = trader.Execution.Fill.FeeUsdEquiv
	trader.Fills++
	trader.Fees = trader.Fees.Add(evaluation.Fee)
	return trader.Recorder.WriteLearning(evaluation)
}

/*
	Objective values the whole wallet with the existing price owner. The return

unit is a fraction of initial funding; the core owns elapsed-time feedback.
*/
func (trader *Trader) Objective() (reward.Mark, error) {
	equity := trader.Balance.Cash()
	realized := decimal.NewFromInt64(0)

	for _, regulator := range trader.Positions {
		realized = realized.Add(regulator.Holding.RealizedPnL)

		if regulator.Holding.Qty.Sign() == 0 {
			continue
		}

		if err := regulator.Mark(trader.At); err != nil {
			return reward.Mark{}, err
		}
		equity = equity.Add(regulator.Surface.ExecutableValue)
	}
	trader.Equity, trader.Profit = equity, equity.Sub(trader.Initial)
	trader.Realized, trader.Unrealized = realized, trader.Profit.Sub(realized)
	trader.Wealth = trader.Profit.Div(trader.Initial).Float64()
	return reward.Mark{At: trader.At, Version: trader.Version, Value: trader.Wealth}, nil
}
