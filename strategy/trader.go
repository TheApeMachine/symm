package strategy

import (
	"time"

	spotbook "github.com/krakenfx/api-go/v2/pkg/book"
	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/hindsight"
	"github.com/theapemachine/symm/types"
)

/*
Independent traders learning to recognise a move before it happens.

Each trader owns one wallet and works the whole universe with it, the way a desk
does: it is woken by the instruments that light up, it decides what to do about
each one, and it carries as many positions at once as its cash allows. Its
capital is its own — one trader's mistake never spends another's money — so the
same market can be met with different choices at the same instant and the
difference between them is attributable.

What a trader is learning is not a price. It is the shape of a development: what
the measured state looked like a moment ago, what it looks like now, and whether
that transition is the one that precedes a move. Every decision it makes is
conditioned on that recent history rather than on the instant alone, which is
what makes "this is starting" expressible at all.

Nothing here judges a decision by its own outcome alone. The tape settles that
later, in Grade, once the move it was reaching for has actually finished
happening.
*/
type traderPosition struct {
	quantity *decimal.Decimal
	spent    *decimal.Decimal
	openedAt time.Time
	openSeq  hindsight.CaptureSequence
}

/*
TraderDecision is one move a trader made, retained until the tape can say
whether it was the right one.

It keeps the development it was conditioned on rather than a reference to it.
The trader will have moved on by the time the tape settles this, and the whole
point of the record is what the trader was looking at when it decided.
*/
type TraderDecision struct {
	Trader   int                       `json:"trader"`
	Symbol   string                    `json:"symbol"`
	Action   LearningAction            `json:"action"`
	At       time.Time                 `json:"at"`
	Sequence hindsight.CaptureSequence `json:"sequence"`
	State    string                    `json:"state"`
	Price    float64                   `json:"price"`
	Quantity string                    `json:"quantity"`

	// Development is the ordered context the decision was conditioned on: the
	// state now, then the states before it. This is the A-to-B the trader
	// claimed to recognise, and it is what gets trained when the tape agrees
	// or disagrees.
	Development []uint64 `json:"development"`

	// Graded marks a decision the tape has already settled.
	Graded bool `json:"graded"`
}

/*
Trader is one independent participant: a wallet, the positions it currently
holds, and the decisions it is still waiting to be judged on.
*/
type Trader struct {
	ID        int                        `json:"id"`
	Cash      *decimal.Decimal           `json:"-"`
	Fees      *decimal.Decimal           `json:"-"`
	Positions map[string]*traderPosition `json:"-"`

	// Open decisions await the tape. Settled ones leave once graded.
	Open []TraderDecision `json:"-"`

	Decisions uint64 `json:"decisions"`
	Fills     uint64 `json:"fills"`
	Graded    uint64 `json:"graded"`

	// Quality is how well this trader's moves have matched what the tape
	// actually offered, and Wealth is what its wallet has done. They are kept
	// apart on purpose: a wallet can drift on an open position that has not
	// resolved, while quality only moves when the tape settles something.
	Quality  float64 `json:"quality"`
	Observed float64 `json:"observed"`
	Wealth   float64 `json:"wealth"`

	price   *broker.Price
	initial *decimal.Decimal
}

/* NewTrader gives one participant its own copy of the same known capital. */
func NewTrader(id int, initial *decimal.Decimal, price *broker.Price) *Trader {
	return &Trader{
		ID:        id,
		Cash:      initial.SetScale(decimal.DefaultScale),
		Fees:      zero,
		Positions: map[string]*traderPosition{},
		price:     price, initial: initial.Copy(),
	}
}

/* Holds reports whether this trader currently carries inventory in a symbol. */
func (trader *Trader) Holds(symbol string) bool {
	position := trader.Positions[symbol]
	return position != nil && position.quantity.Sign() > 0
}

/* State names the account state a decision on this symbol is conditioned in. */
func (trader *Trader) State(symbol string) string {
	if trader.Holds(symbol) {
		return "holding"
	}

	return "flat"
}

/*
Feasible lists what this trader could actually do in one symbol right now.

The set is bounded by its own wallet and by the venue: there is no point
offering an exit to a trader holding nothing, and no point offering an entry it
could not pay for. A trader is never asked to choose among moves it cannot make.
*/
func (trader *Trader) Feasible(
	symbol string, book *spotbook.Book, output []LearningAction,
) ([]LearningAction, error) {
	output = append(output[:0], LearningAction{Kind: types.ActionHold})
	pair := trader.price.Instrument.Pair(symbol)

	if pair.QtyIncrement == nil || pair.QtyIncrement.Sign() <= 0 {
		return output, nil
	}

	holding := trader.Holds(symbol)

	if buyable, err := trader.affordable(symbol, book); err != nil {
		return nil, err
	} else if buyable != nil {
		kind := types.ActionEnter

		if holding {
			kind = types.ActionScale
		}

		output = trader.bisect(output, symbol, buyable, book.BestAsk().Price, kind, false, pair.QtyIncrement)
	}

	if holding {
		held := trader.Positions[symbol].quantity
		output = trader.bisect(output, symbol, held, book.BestBid().Price, types.ActionExit, true, pair.QtyIncrement)
	}

	return output, nil
}

/*
bisect enumerates halvings of a reachable quantity down to the venue minimum, so
a trader can express size as well as direction.
*/
func (trader *Trader) bisect(
	output []LearningAction,
	symbol string,
	quantity, unit *decimal.Decimal,
	kind types.Action,
	reduce bool,
	increment *decimal.Decimal,
) []LearningAction {
	previous := zero

	for power := uint16(0); trader.price.Tradable(symbol, quantity, unit); power++ {
		action := LearningAction{Kind: kind, Power: power, Reduce: reduce}

		if reduce && power > 0 {
			action.Kind = types.ActionScale
		}

		if quantity.Cmp(previous) != 0 {
			output = append(output, action)
		}

		previous = quantity
		quantity = quantity.Div(two).SetSize(increment)
	}

	return output
}

/*
affordable is the largest quantity this wallet could buy at the current touch.
*/
func (trader *Trader) affordable(
	symbol string, book *spotbook.Book,
) (*decimal.Decimal, error) {
	if trader.Cash.Sign() <= 0 {
		return nil, nil
	}

	requested, err := trader.price.Affordable(symbol, trader.Cash, book.BestAsk().Price)

	if err != nil || requested == nil || requested.Sign() <= 0 {
		return nil, err
	}

	quantity, _, err := trader.price.Walk(book, requested, broker.BUY)

	if quantity == nil {
		return nil, err
	}

	return quantity, nil
}

/*
Execute applies one chosen move to this trader's own wallet against the
displayed book, and returns the decision it created.

A move that the book cannot fill is not recorded as a decision. The trader
reached for something the market did not offer, and inventing a fill would put
a position in the wallet that the venue never gave it.
*/
func (trader *Trader) Execute(
	symbol string,
	action LearningAction,
	book *spotbook.Book,
	development []uint64,
	at time.Time,
	sequence hindsight.CaptureSequence,
) (TraderDecision, bool, error) {
	decision := TraderDecision{
		Trader:      trader.ID,
		Symbol:      symbol,
		Action:      action,
		At:          at,
		Sequence:    sequence,
		State:       trader.State(symbol),
		Development: append([]uint64(nil), development...),
	}

	trader.Decisions++

	if action.Kind == types.ActionHold {
		decision.Price = midpoint(book)
		trader.Open = append(trader.Open, decision)

		return decision, true, nil
	}
	quantity, err := trader.requested(symbol, action, book)

	if err != nil {
		return decision, false, err
	}

	if quantity == nil || quantity.Sign() <= 0 {
		return decision, false, nil
	}

	if action.Reduce {
		return trader.reduce(symbol, book, quantity, decision)
	}

	return trader.acquire(symbol, book, quantity, decision)
}

/* requested sizes the chosen move from this wallet's own reach. */
func (trader *Trader) requested(
	symbol string, action LearningAction, book *spotbook.Book,
) (*decimal.Decimal, error) {
	pair := trader.price.Instrument.Pair(symbol)

	if pair.QtyIncrement == nil || pair.QtyIncrement.Sign() <= 0 {
		return nil, nil
	}
	quantity := (*decimal.Decimal)(nil)

	if action.Reduce {
		position := trader.Positions[symbol]

		if position == nil {
			return nil, nil
		}
		quantity = position.quantity
	} else {
		affordable, err := trader.affordable(symbol, book)

		if err != nil || affordable == nil {
			return nil, err
		}
		quantity = affordable
	}

	for range action.Power {
		quantity = quantity.Div(two).SetSize(pair.QtyIncrement)
	}

	return quantity, nil
}

/* acquire buys into the book and records what the wallet actually paid. */
func (trader *Trader) acquire(
	symbol string,
	book *spotbook.Book,
	requested *decimal.Decimal,
	decision TraderDecision,
) (TraderDecision, bool, error) {
	if !trader.price.Tradable(symbol, requested, book.BestAsk().Price) {
		return decision, false, nil
	}
	quantity, gross, err := trader.price.Walk(book, requested, broker.BUY)

	if quantity == nil || quantity.Sign() <= 0 {
		return decision, false, err
	}
	total := trader.price.WithFee(symbol, gross, broker.BUY)

	if total == nil || total.Cmp(trader.Cash) > 0 {
		return decision, false, nil
	}
	trader.Fees = trader.Fees.Add(total.Sub(gross).Abs())
	trader.Cash = trader.Cash.Sub(total)
	position := trader.Positions[symbol]

	if position == nil {
		position = &traderPosition{quantity: zero, spent: zero, openedAt: decision.At, openSeq: decision.Sequence}
		trader.Positions[symbol] = position
	}
	position.quantity = position.quantity.Add(quantity)
	position.spent = position.spent.Add(total)
	trader.Fills++

	decision.Quantity, decision.Price = quantity.String(), book.BestAsk().Price.Float64()
	trader.Open = append(trader.Open, decision)

	return decision, true, nil
}

/* reduce sells inventory back into the book at the displayed bid. */
func (trader *Trader) reduce(
	symbol string,
	book *spotbook.Book,
	requested *decimal.Decimal,
	decision TraderDecision,
) (TraderDecision, bool, error) {
	position := trader.Positions[symbol]

	if position == nil || !trader.price.Tradable(symbol, requested, book.BestBid().Price) {
		return decision, false, nil
	}

	if requested.Cmp(position.quantity) > 0 {
		requested = position.quantity
	}
	quantity, gross, err := trader.price.Walk(book, requested, broker.SELL)

	if quantity == nil || quantity.Sign() <= 0 {
		return decision, false, err
	}
	total := trader.price.WithFee(symbol, gross, broker.SELL)

	if total == nil {
		return decision, false, nil
	}
	trader.Fees = trader.Fees.Add(total.Sub(gross).Abs())
	trader.Cash = trader.Cash.Add(total)
	position.quantity = position.quantity.Sub(quantity)
	position.spent = position.spent.Sub(total)
	trader.Fills++

	if position.quantity.Sign() <= 0 {
		delete(trader.Positions, symbol)
	}

	decision.Quantity, decision.Price = quantity.String(), book.BestBid().Price.Float64()
	trader.Open = append(trader.Open, decision)

	return decision, true, nil
}

/*
Mark values this wallet at what it could actually liquidate for right now, and
folds the result into the trader's wealth reading.

Wealth is a weaker signal than the tape's own verdict and is treated as one: it
moves continuously, including on positions that have not resolved into anything
yet, so it says how the wallet is doing rather than whether a decision was
right.
*/
func (trader *Trader) Mark(books LearningBook) {
	equity := trader.Cash.Copy()

	for symbol, position := range trader.Positions {
		if position.quantity.Sign() <= 0 {
			continue
		}
		books.Book(symbol, func(book *spotbook.Book) {
			if book == nil || book.Bids == nil || book.Bids.High == nil {
				return
			}
			quantity, gross, err := trader.price.Walk(book, position.quantity, broker.SELL)

			if err != nil || quantity == nil || quantity.Cmp(position.quantity) != 0 {
				return
			}

			if total := trader.price.WithFee(symbol, gross, broker.SELL); total != nil {
				equity = equity.Add(total)
			}
		})
	}
	initial := trader.initial.Float64()

	if initial > 0 {
		trader.Wealth = (equity.Float64() - initial) / initial
	}
}

/* midpoint is the executable middle of the displayed book. */
func midpoint(book *spotbook.Book) float64 {
	if book == nil || book.Bids == nil || book.Asks == nil ||
		book.Bids.High == nil || book.Asks.Low == nil {
		return 0
	}

	return (book.Bids.High.Price.Float64() + book.Asks.Low.Price.Float64()) / 2
}

/* Error surfaces a wallet fault to the owning learner. */
func (trader *Trader) Error() error {
	if trader.Cash != nil && trader.Cash.Sign() < 0 {
		return errnie.Err(errnie.Internal, "trader: wallet overdrawn", nil)
	}

	return nil
}
