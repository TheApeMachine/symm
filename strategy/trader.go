package strategy

import (
	"fmt"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/book"
	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/broker/position"
	"github.com/theapemachine/symm/hindsight/recording"
	"github.com/theapemachine/symm/hindsight/tables"
	"github.com/theapemachine/symm/kraken/websocket"
	"github.com/theapemachine/symm/nomagique/learning/associative/agent"
	"github.com/theapemachine/symm/nomagique/learning/associative/reward"
	"golang.org/x/sync/errgroup"
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

// FlatPositionContext distinguishes an unheld position from held inventory.
const FlatPositionContext uint64 = 1 << 63

/*
	Trader adapts an independent account and its normal position regulators to

associative learning. It is owned by Learner's serialized state transitions.
*/
type Trader struct {
	Balance                                       *broker.Balance
	Positions                                     map[string]*position.Regulator
	price                                         *broker.Price
	Initial, Equity, Profit, Realized, Unrealized *decimal.Decimal
	Wealth                                        float64
	At                                            time.Time
	Version, Fills                                uint64
	Status                                        string
	Alternatives                                  []Action
	Quantities                                    map[Action]*decimal.Decimal
	Evaluations                                   map[string]map[uint64]*Evaluation
	Opened                                        map[string]time.Time
	Ending                                        map[string]time.Time
	LastEvaluation                                *Evaluation
	Recorder                                      *recording.Session
	ID                                            int
	Fees                                          *decimal.Decimal
	api                                           *websocket.API
}

func NewTrader(
	api *websocket.API,
	price *broker.Price,
	quote string,
	cash *decimal.Decimal,
) (*Trader, error) {
	balance, err := broker.NewFundedBalance(quote, cash)

	if err != nil {
		return nil, err
	}

	return &Trader{
		Status:      "waiting for regions",
		api:         api,
		Balance:     balance,
		price:       price,
		Initial:     cash,
		Equity:      cash,
		Realized:    decimal.NewFromInt64(0),
		Unrealized:  decimal.NewFromInt64(0),
		Fees:        decimal.NewFromInt64(0),
		Profit:      decimal.NewFromInt64(0),
		Positions:   make(map[string]*position.Regulator),
		Opened:      make(map[string]time.Time),
		Ending:      make(map[string]time.Time),
		Evaluations: make(map[string]map[uint64]*Evaluation),
	}, nil
}

/*
	Feasible enumerates choices constrained only by owned resources and venue

execution requirements. Unavailable inputs are errors, never wait/hold evidence.
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
	trader.price.Books.Book(symbol, func(book *book.Book) {
		if book != nil && book.BestBid() != nil && book.BestAsk() != nil {
			bid, ask = book.BestBid().Price, book.BestAsk().Price
		}
	})
	state := []uint64{FlatPositionContext}

	if held.Sign() > 0 {
		state[0]++
	}

	if bid == nil || ask == nil || trader.price.FeeIfAvailable(symbol) == nil {
		return nil, nil, errnie.Error(errnie.Err(errnie.Conflict, "trader: market dependencies are not ready", nil))
	}
	trader.Status = "learning"

	if held.Sign() > 0 {
		if err := trader.sizes(symbol, Action{Kind: "exit", Reduce: true}, held, bid); err != nil {
			return nil, nil, err
		}

	}

	if trader.Balance.Cash().Sign() <= 0 {
		return trader.Alternatives, state, nil
	}
	kind := "enter"

	if held.Sign() > 0 {
		kind = "scale"
	}

	quantity, err := trader.price.Affordable(symbol, trader.Balance.Cash(), ask)

	if err != nil {
		return nil, nil, err
	}

	if err := trader.sizes(symbol, Action{Kind: kind}, quantity, ask); err != nil {
		return nil, nil, err
	}
	return trader.Alternatives, state, nil
}

/* open counts the symbols this wallet currently holds inventory in. */
func (trader *Trader) open() int {
	count := 0

	for _, regulator := range trader.Positions {
		if regulator.Holding.Qty.Sign() > 0 {
			count++
		}
	}

	return count
}

/*
	sizes enumerates the binary resource fractions down to the venue's minimum.

No fixed exposure cap or prediction threshold participates.
*/
func (trader *Trader) sizes(symbol string, action Action, quantity, unit *decimal.Decimal) error {
	for trader.price.Tradable(symbol, quantity, unit) {
		usable := false

		if action.Reduce {
			surface, err := trader.price.Surface(symbol, quantity, trader.At)
			usable = err == nil && surface.FullyExecutable

			if err != nil {
				trader.Status = err.Error()

				if !errnie.IsUnprocessableContent(err) {
					return err
				}
			}
		}

		if !action.Reduce {
			cost, err := trader.price.EntryCost(symbol, quantity)
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

	if trader.Evaluations[decision.Label] == nil {
		trader.Evaluations[decision.Label] = make(map[uint64]*Evaluation)
	}
	trader.Evaluations[decision.Label][decision.ID] = evaluation
	regulator := trader.Positions[decision.Label]
	evaluation.Quantity = decimal.NewFromInt64(0)

	if regulator != nil {
		evaluation.Quantity = regulator.Holding.Qty
	}
	trader.price.Books.Book(decision.Label, func(current *book.Book) {
		if current != nil && current.BestBid() != nil {
			evaluation.Reference = current.BestBid().Price
		}
	})

	if decision.Action.Kind == "wait" || decision.Action.Kind == "hold" {
		for _, action := range trader.Alternatives {
			if action.Kind == "enter" && evaluation.Opportunity == nil {
				evaluation.Opportunity = trader.Quantities[action]
			}
		}

		if trader.Recorder != nil {
			return trader.Recorder.WriteDecision(evaluation.Row(""))
		}
		return nil
	}

	if regulator == nil {
		regulator = position.NewPaperRegulator(trader.api, trader.price, trader.Balance, decision.Label)
		trader.Positions[decision.Label] = regulator
	}
	side := broker.BUY

	if decision.Action.Reduce {
		side = broker.SELL
	}
	regulator.At = decision.At

	if err := regulator.Submit(fmt.Sprint(decision.ID), side, trader.Quantities[decision.Action]); err != nil {
		return err
	}

	evaluation.Quantity = regulator.LastExecution.CumQty
	evaluation.Cost = regulator.LastExecution.CumCost
	evaluation.Fee = regulator.LastExecution.FeeUsdEquiv
	trader.mark(decision.Label, regulator, decision.At)
	trader.Fills++
	trader.Fees = trader.Fees.Add(evaluation.Fee)

	if trader.Recorder != nil {
		return trader.Recorder.WriteDecision(evaluation.Row(""))
	}
	return nil
}

/*
mark tracks when the current position in a symbol was established. It is stamped
on the fill that opens one and cleared when the symbol goes flat, so holding age
belongs to the position actually held rather than to the symbol's whole history.
*/
func (trader *Trader) mark(symbol string, regulator *position.Regulator, at time.Time) {
	if regulator.Holding.Qty.Sign() == 0 {
		delete(trader.Opened, symbol)
		return
	}

	if _, exists := trader.Opened[symbol]; !exists {
		trader.Opened[symbol] = at
	}
}

/*
	Objective values the whole wallet with the existing price owner. The return

unit is a fraction of initial funding; the core owns elapsed-time feedback.
A missing held book leaves the mark absent and the last valuation unchanged.
*/
func (trader *Trader) Objective() (*reward.Mark, error) {
	// Each worker owns one regulator and result slot. Wallet state is committed
	// only after all workers finish, including when a book cannot be valued.
	marks := make([]struct {
		regulator *position.Regulator
		status    string
	}, len(trader.Positions))
	index := 0

	for _, regulator := range trader.Positions {
		marks[index].regulator = regulator
		index++
	}
	var group errgroup.Group

	for index := range marks {
		regulator := marks[index].regulator

		if regulator.Holding.Qty.Sign() == 0 {
			continue
		}
		group.Go(func() error {
			available := false
			trader.price.Books.Book(regulator.Holding.Symbol, func(book *book.Book) {
				available = book != nil && book.BestBid() != nil && book.BestAsk() != nil
			})

			if !available {
				marks[index].status = "awaiting book: " + regulator.Holding.Symbol
				return nil
			}

			if err := regulator.Mark(trader.At); err != nil {
				// Missing, crossed or insufficient depth leaves valuation absent.
				// Unexpected failures still fail the whole measurement phase.
				if errnie.IsNotFound(err) || errnie.IsUnprocessableContent(err) || errnie.IsValidation(err) {
					marks[index].status = "unmarkable: " + regulator.Holding.Symbol
					return nil
				}
				return errnie.Error(err)
			}

			if regulator.Surface == nil {
				marks[index].status = "unmarkable: " + regulator.Holding.Symbol
			}
			return nil
		})
	}

	if err := group.Wait(); err != nil {
		return nil, errnie.Error(err)
	}
	equity := trader.Balance.Cash().SetScale(decimal.DefaultScale)
	realized := decimal.NewFromInt64(0)

	for _, mark := range marks {
		if mark.status != "" {
			trader.Status = mark.status
			return nil, nil
		}
		regulator := mark.regulator
		realized = realized.Add(regulator.Holding.RealizedPnL)

		if regulator.Holding.Qty.Sign() > 0 {
			equity = equity.Add(regulator.Surface.ExecutableValue)
		}
	}
	trader.Equity, trader.Profit = equity, equity.Sub(trader.Initial)
	trader.Realized, trader.Unrealized = realized, trader.Profit.Sub(realized)
	trader.Wealth = trader.Profit.Div(trader.Initial).Float64()

	trader.Status = "learning"

	return &reward.Mark{
		At: trader.At, Version: trader.Version, Value: trader.Wealth,
	}, nil
}

// Ready tests the resident dependencies used by Feasible without issuing any
// decision or mutating learning state. Reseeding books are hidden by API.Book.
func (trader *Trader) Ready(symbol string) bool {
	ready := false
	trader.price.Books.Book(symbol, func(current *book.Book) {
		ready = current != nil && current.BestBid() != nil && current.BestAsk() != nil
	})
	return ready && trader.price.FeeIfAvailable(symbol) != nil
}

// End releases inventory whose opportunity has been evaluated. This is an
// environment boundary, never another agent choice or a learned timeout.
// Unavailable depth leaves the release pending until a later ready book.
func (trader *Trader) End(symbol string, through time.Time) error {
	opened, held := trader.Opened[symbol]

	if !held || opened.After(through) {
		delete(trader.Ending, symbol)
		return nil
	}
	trader.Ending[symbol] = through

	if !trader.Ready(symbol) {
		return nil
	}
	regulator := trader.Positions[symbol]
	surface, err := trader.price.Surface(symbol, regulator.Holding.Qty, trader.At)

	if err != nil {
		if errnie.IsNotFound(err) || errnie.IsUnprocessableContent(err) {
			return nil
		}
		return errnie.Error(err)
	}

	if !surface.FullyExecutable {
		return nil
	}
	regulator.At = trader.At

	if err := regulator.Submit("evaluation/"+through.Format(time.RFC3339Nano), broker.SELL, regulator.Holding.Qty); err != nil {
		return errnie.Error(err)
	}
	fill := regulator.LastExecution
	trader.Fills++
	trader.Fees = trader.Fees.Add(fill.FeeUsdEquiv)
	trader.mark(symbol, regulator, trader.At)
	delete(trader.Ending, symbol)

	if trader.Recorder != nil {
		return trader.Recorder.WriteDecision(tables.OutcomeRow{
			Trader: int32(trader.ID), DecisionID: -through.UnixNano(), Label: symbol,
			At: trader.At, Through: through, ActionKind: "evaluation_exit", ActionReduce: true,
			Forced: true, Initial: trader.Initial, Quantity: fill.CumQty,
			Cost: fill.CumCost, Fee: fill.FeeUsdEquiv,
		})
	}
	return nil
}
