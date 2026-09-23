package paper

import (
	"bytes"
	"encoding/json"

	"github.com/krakenfx/api-go/v2/pkg/book"
	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/krakenfx/api-go/v2/pkg/spot"
	"github.com/theapemachine/errnie"
)

const (
	sideBuy  = "buy"
	sideSell = "sell"

	// Reasons an order resolved without executing. The first is the
	// exchange's own refusal; the others mean the replay cannot say what
	// would have happened, so the outcome is unknown rather than a loss.
	reasonBelowMinimum = "below_minimum"
	reasonNoLiquidity  = "no_liquidity"
	reasonNoInstrument = "unknown_instrument"
)

/* rules are the exchange's order constraints for one pair. */
type rules struct {
	minimumQuantity *decimal.Decimal
	minimumCost     *decimal.Decimal
	increment       *decimal.Decimal
}

/* order is a market order: a buy spends a quote amount, a sell delivers a base quantity. */
type order struct {
	id     uint64
	symbol string
	side   string
	amount *decimal.Decimal
}

type execution struct {
	order                      uint64
	symbol, side, reason, time string
	quantity, cost, unfilled   *decimal.Decimal
}

/* venue owns the replayed books, the pair rules and the orders resting on them. */
type venue struct {
	books     map[string]*book.Book
	rules     map[string]rules
	resting   []order
	reconcile *spot.BookManager
}

func newVenue() *venue {
	return &venue{
		books:     make(map[string]*book.Book),
		rules:     make(map[string]rules),
		reconcile: spot.NewBookManager(),
	}
}

func (market *venue) rest(placed order) error {
	for _, held := range market.resting {
		if held.id == placed.id {
			return errnie.Error(errnie.Err(errnie.Validation, "venue: order already resting", nil))
		}
	}
	market.resting = append(market.resting, placed)
	return nil
}

/* apply advances the books by one frame and executes what rested on the symbols it moved. */
func (market *venue) apply(frame []byte, depth int64, moment string) ([]execution, error) {
	decoder := json.NewDecoder(bytes.NewReader(frame))
	decoder.UseNumber()

	var envelope struct {
		Channel string          `json:"channel"`
		Type    string          `json:"type"`
		Data    json.RawMessage `json:"data"`
	}

	if err := decoder.Decode(&envelope); err != nil {
		return nil, errnie.Error(errnie.Err(errnie.Validation, "venue: decode frame", err))
	}

	if envelope.Channel == "instrument" {
		return nil, market.instruments(envelope.Data)
	}

	if envelope.Channel != "level3" {
		return nil, nil
	}

	if depth <= 0 {
		return nil, errnie.Error(errnie.Err(errnie.Validation, "venue: level3 frames need the subscribed depth", nil))
	}
	decoder = json.NewDecoder(bytes.NewReader(envelope.Data))
	decoder.UseNumber()
	var entries []map[string]any

	if err := decoder.Decode(&entries); err != nil {
		return nil, errnie.Error(errnie.Err(errnie.Validation, "venue: decode level3 data", err))
	}
	var executed []execution

	for _, entry := range entries {
		symbol, found := entry["symbol"].(string)

		if !found || symbol == "" {
			return nil, errnie.Error(errnie.Err(errnie.Validation, "venue: level3 entry without symbol", nil))
		}

		if envelope.Type == "snapshot" {
			fresh := book.New()
			fresh.Name = symbol
			fresh.MaxDepth = int(depth)
			fresh.EnableMaxDepth = true
			market.books[symbol] = fresh
		}
		held := market.books[symbol]

		if held == nil {
			continue
		}

		// A book that no longer reconciles with the exchange's checksum is
		// not the book the exchange held; it stays unknown until a snapshot.
		if err := market.reconcile.UpdateL3(held, entry); err != nil {
			errnie.Warn("venue: book lost until next snapshot", "symbol", symbol, "cause", err.Error())
			delete(market.books, symbol)
			continue
		}
		filled, err := market.fill(symbol, held, moment)

		if err != nil {
			return nil, err
		}
		executed = append(executed, filled...)
	}
	return executed, nil
}

func (market *venue) instruments(data json.RawMessage) error {
	var catalog struct {
		Pairs []struct {
			Symbol          string      `json:"symbol"`
			MinimumQuantity json.Number `json:"qty_min"`
			MinimumCost     json.Number `json:"cost_min"`
			Increment       json.Number `json:"qty_increment"`
		} `json:"pairs"`
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()

	if err := decoder.Decode(&catalog); err != nil {
		return errnie.Error(errnie.Err(errnie.Validation, "venue: decode instrument data", err))
	}

	for _, pair := range catalog.Pairs {
		quantity, err := amount(pair.MinimumQuantity.String(), pair.Symbol+" qty_min")

		if err != nil {
			return err
		}
		cost, err := decimal.NewFromString(pair.MinimumCost.String())

		if err != nil {
			return errnie.Error(errnie.Err(errnie.Validation, "venue: "+pair.Symbol+" cost_min", err))
		}
		increment, err := amount(pair.Increment.String(), pair.Symbol+" qty_increment")

		if err != nil {
			return err
		}
		market.rules[pair.Symbol] = rules{minimumQuantity: quantity, minimumCost: cost, increment: increment}
	}
	return nil
}

/* fill executes every order that was resting on symbol before this frame. */
func (market *venue) fill(symbol string, held *book.Book, moment string) ([]execution, error) {
	remaining := make([]order, 0, len(market.resting))
	var executed []execution

	for _, resting := range market.resting {
		if resting.symbol != symbol {
			remaining = append(remaining, resting)
			continue
		}
		result, err := market.execute(resting, held, moment)

		if err != nil {
			return nil, err
		}
		executed = append(executed, result)
	}
	market.resting = remaining
	return executed, nil
}

func (market *venue) execute(resting order, held *book.Book, moment string) (execution, error) {
	result := execution{
		order: resting.id, symbol: resting.symbol, side: resting.side, time: moment,
		quantity: zero(), cost: zero(), unfilled: resting.amount,
	}
	pair, found := market.rules[resting.symbol]

	if !found {
		result.reason = reasonNoInstrument
		return result, nil
	}
	var quantity, cost, unfilled *decimal.Decimal
	var err error

	switch resting.side {
	case sideBuy:
		quantity, cost, unfilled, err = walkAsks(held, resting.amount, pair.increment)
	case sideSell:
		quantity, cost, unfilled, err = walkBids(held, resting.amount)
	}

	if err != nil {
		return result, err
	}

	if quantity.Sign() == 0 {
		result.reason = reasonNoLiquidity
		return result, nil
	}

	if quantity.Cmp(pair.minimumQuantity) < 0 || cost.Cmp(pair.minimumCost) < 0 {
		result.reason = reasonBelowMinimum
		return result, nil
	}
	result.quantity, result.cost, result.unfilled = quantity, cost, unfilled
	return result, nil
}

/* walkAsks spends a quote budget up the ask side in whole increments. */
func walkAsks(held *book.Book, budget, increment *decimal.Decimal) (quantity, cost, unfilled *decimal.Decimal, err error) {
	quantity, cost, unfilled = zero(), zero(), budget

	for level := held.BestAsk(); level != nil && unfilled.Sign() > 0; level = level.Higher {
		levelCost, err := times(level.Price, level.Quantity)

		if err != nil {
			return quantity, cost, unfilled, err
		}

		if levelCost.Cmp(unfilled) <= 0 {
			quantity = plus(quantity, level.Quantity)
			cost = plus(cost, levelCost)
			unfilled = minus(unfilled, levelCost)
			continue
		}
		take, err := floorTo(unfilled, level.Price, increment)

		if err != nil {
			return quantity, cost, unfilled, err
		}

		if take.Sign() == 0 {
			break
		}
		spent, err := times(level.Price, take)

		if err != nil {
			return quantity, cost, unfilled, err
		}
		quantity = plus(quantity, take)
		cost = plus(cost, spent)
		unfilled = minus(unfilled, spent)
		break
	}
	return quantity, cost, unfilled, nil
}

/* walkBids delivers a base quantity down the bid side. */
func walkBids(held *book.Book, delivery *decimal.Decimal) (quantity, cost, unfilled *decimal.Decimal, err error) {
	quantity, cost, unfilled = zero(), zero(), delivery

	for level := held.BestBid(); level != nil && unfilled.Sign() > 0; level = level.Lower {
		take := level.Quantity

		if take.Cmp(unfilled) > 0 {
			take = unfilled
		}
		earned, err := times(level.Price, take)

		if err != nil {
			return quantity, cost, unfilled, err
		}
		quantity = plus(quantity, take)
		cost = plus(cost, earned)
		unfilled = minus(unfilled, take)
	}
	return quantity, cost, unfilled, nil
}
