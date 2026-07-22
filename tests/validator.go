package tests

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"time"
)

/*
Validator reconstructs the emitted venue state and rejects cross-feed conflicts
before any fixture payload reaches the production connection boundary.
*/
type Validator struct {
	books        map[string]map[string]map[float64]float64
	orders       map[string]map[string]map[string]orderState
	ticker       map[string]tickerState
	observed     map[string]time.Time
	nextPriority uint64
}

/*
NewValidator creates an empty invariant gate for one deterministic venue.
*/
func NewValidator() *Validator {
	return &Validator{
		books:    map[string]map[string]map[float64]float64{},
		orders:   map[string]map[string]map[string]orderState{},
		ticker:   map[string]tickerState{},
		observed: map[string]time.Time{},
	}
}

/*
Validate applies one coherent frame set and verifies its economic invariants.
*/
func (validator *Validator) Validate(payloads frameSet) error {
	draft := validator.clone()

	if err := draft.validate(payloads); err != nil {
		return err
	}

	*validator = *draft
	return nil
}

/*
validate applies a frame set only to an isolated validator draft.
*/
func (validator *Validator) validate(payloads frameSet) error {
	var ticker wireFrame[wireTicker]
	var trades wireFrame[wireTrade]
	var book wireFrame[wireBook]
	var level3 wireFrame[wireLevel3]

	for _, input := range []struct {
		channel string
		payload []byte
		frame   any
	}{
		{"ticker", payloads.ticker, &ticker},
		{"trade", payloads.trade, &trades},
		{"book", payloads.book, &book},
		{"level3", payloads.level3, &level3},
	} {
		if len(input.payload) == 0 {
			continue
		}

		header := wireFrame[json.RawMessage]{}

		if err := json.Unmarshal(input.payload, &header); err != nil {
			return fmt.Errorf("tests: decode simulated %s envelope: %w", input.channel, err)
		}

		if header.Channel != input.channel ||
			header.Type != "snapshot" && header.Type != "update" ||
			len(header.Data) == 0 {
			return fmt.Errorf("tests: invalid simulated %s envelope", input.channel)
		}

		decoder := json.NewDecoder(bytes.NewReader(input.payload))
		decoder.UseNumber()

		if err := decoder.Decode(input.frame); err != nil {
			return fmt.Errorf("tests: decode simulated frame: %w", err)
		}
	}
	if err := validator.validateTimes(ticker.Data, trades.Data, book.Data, level3.Data); err != nil {
		return err
	}

	tradesBySymbol := make(map[string][]wireTrade, len(trades.Data))

	for _, trade := range trades.Data {
		tradesBySymbol[trade.Symbol] = append(tradesBySymbol[trade.Symbol], trade)
	}

	for symbol, executions := range tradesBySymbol {
		if validator.orders[symbol] == nil {
			continue
		}

		if err := validator.validateExecutions(symbol, executions); err != nil {
			return err
		}
	}

	level3Symbols := make(map[string]bool, len(level3.Data))

	for _, row := range level3.Data {
		if err := validator.applyLevel3(level3.Type, row); err != nil {
			return err
		}

		level3Symbols[row.Symbol] = true
	}

	for _, row := range book.Data {
		if !level3Symbols[row.Symbol] {
			return fmt.Errorf("tests: L2 update missing matching L3 state for %s", row.Symbol)
		}

		if err := validator.applyBook(book.Type, row); err != nil {
			return err
		}

		if err := validator.validateAggregates(row.Symbol); err != nil {
			return err
		}
	}

	tickerSymbols := make(map[string]bool, len(ticker.Data))

	for _, row := range ticker.Data {
		if err := validator.validateSymbol(row, tradesBySymbol[row.Symbol]); err != nil {
			return err
		}

		tickerSymbols[row.Symbol] = true
	}

	for symbol := range tradesBySymbol {
		if !tickerSymbols[symbol] {
			return fmt.Errorf("tests: trade update missing ticker for %s", symbol)
		}
	}

	return nil
}

/*
applyLevel3 reconstructs order lifecycles and verifies the resulting checksum.
*/
func (validator *Validator) applyLevel3(frameType string, row wireLevel3) error {
	if frameType == "snapshot" || validator.orders[row.Symbol] == nil {
		validator.orders[row.Symbol] = map[string]map[string]orderState{
			"bids": {},
			"asks": {},
		}
	}

	for _, input := range []struct {
		side   string
		orders []wireOrder
	}{
		{"bids", row.Bids},
		{"asks", row.Asks},
	} {
		side := input.side
		orders := input.orders

		for _, order := range orders {
			current := validator.orders[row.Symbol][side]
			_, exists := current[order.OrderID]
			price, err := order.Price.Float64()

			if err != nil {
				return fmt.Errorf(
					"tests: invalid L3 price for %s: %w",
					order.OrderID,
					err,
				)
			}

			quantity, err := order.Qty.Float64()

			if err != nil {
				return fmt.Errorf(
					"tests: invalid L3 quantity for %s: %w",
					order.OrderID,
					err,
				)
			}

			switch order.Event {
			case "":
				if frameType != "snapshot" {
					return fmt.Errorf("tests: L3 update missing event for %s", order.OrderID)
				}
			case "add":
				if exists {
					return fmt.Errorf("tests: duplicate L3 add for %s", order.OrderID)
				}
			case "modify":
				if !exists {
					return fmt.Errorf("tests: L3 modify before add for %s", order.OrderID)
				}
			case "delete":
				if !exists {
					return fmt.Errorf("tests: L3 delete before add for %s", order.OrderID)
				}

				delete(current, order.OrderID)
				continue
			default:
				return fmt.Errorf("tests: unknown L3 event %q", order.Event)
			}

			priority := current[order.OrderID].priority

			if !exists {
				validator.nextPriority++
				priority = validator.nextPriority
			}

			current[order.OrderID] = orderState{
				id:         order.OrderID,
				price:      order.Price.String(),
				qty:        order.Qty.String(),
				priceValue: price,
				qtyValue:   quantity,
				priority:   priority,
			}
		}
	}

	if validator.level3Checksum(row.Symbol) != row.Checksum {
		return fmt.Errorf("tests: invalid L3 checksum for %s", row.Symbol)
	}

	return nil
}

/*
applyBook reconstructs Level2 deltas and verifies the resulting checksum.
*/
func (validator *Validator) applyBook(frameType string, row wireBook) error {
	if frameType == "snapshot" || validator.books[row.Symbol] == nil {
		validator.books[row.Symbol] = map[string]map[float64]float64{
			"bids": {},
			"asks": {},
		}
	}

	for _, input := range []struct {
		side   string
		levels []wireLevel
	}{
		{"bids", row.Bids},
		{"asks", row.Asks},
	} {
		side := input.side
		levels := input.levels

		for _, level := range levels {
			price, err := level.Price.Float64()

			if err != nil {
				return fmt.Errorf("tests: invalid L2 price for %s: %w", row.Symbol, err)
			}

			quantity, err := level.Qty.Float64()

			if err != nil {
				return fmt.Errorf("tests: invalid L2 quantity for %s: %w", row.Symbol, err)
			}

			if quantity == 0 {
				delete(validator.books[row.Symbol][side], price)
				continue
			}

			validator.books[row.Symbol][side][price] = quantity
		}
	}

	if validator.bookChecksum(row.Symbol) != row.Checksum {
		return fmt.Errorf("tests: invalid L2 checksum for %s", row.Symbol)
	}

	return nil
}

/*
validateSymbol checks one symbol's cross-feed touch and trade invariants.
*/
func (validator *Validator) validateSymbol(
	ticker wireTicker,
	trades []wireTrade,
) error {
	if len(trades) > 0 && ticker.Symbol != trades[0].Symbol {
		return fmt.Errorf("tests: simulated frame symbols differ")
	}

	symbol := ticker.Symbol
	bid, bidQty := validator.touch(symbol, "bids", true)
	ask, askQty := validator.touch(symbol, "asks", false)
	tickerBid, err := ticker.Bid.Float64()

	if err != nil {
		return fmt.Errorf("tests: invalid ticker bid for %s: %w", symbol, err)
	}

	tickerAsk, err := ticker.Ask.Float64()

	if err != nil {
		return fmt.Errorf("tests: invalid ticker ask for %s: %w", symbol, err)
	}

	tickerLast, err := ticker.Last.Float64()

	if err != nil {
		return fmt.Errorf("tests: invalid ticker last for %s: %w", symbol, err)
	}

	if bid >= ask || math.Abs(tickerBid-bid) > 1e-8 ||
		math.Abs(tickerAsk-ask) > 1e-8 ||
		math.Abs(ticker.BidQty-bidQty) > 1e-8 ||
		math.Abs(ticker.AskQty-askQty) > 1e-8 {
		return fmt.Errorf("tests: ticker and book touch disagree for %s", symbol)
	}

	if len(trades) == 0 {
		return validator.validateTicker(ticker, nil)
	}

	tradePrice, err := trades[len(trades)-1].Price.Float64()

	if err != nil {
		return fmt.Errorf("tests: invalid trade price for %s: %w", symbol, err)
	}

	if math.Abs(tickerLast-tradePrice) > 1e-8 {
		return fmt.Errorf("tests: ticker last and trade price disagree for %s", symbol)
	}

	return validator.validateTicker(ticker, trades)
}

/*
validateAggregates proves Level2 exactly aggregates the reconstructed Level3.
*/
func (validator *Validator) validateAggregates(symbol string) error {
	aggregated := map[string]map[float64]float64{"bids": {}, "asks": {}}

	for side, orders := range validator.orders[symbol] {
		for _, order := range orders {
			aggregated[side][order.priceValue] += order.qtyValue
		}
	}

	for _, side := range []string{"bids", "asks"} {
		if len(aggregated[side]) != len(validator.books[symbol][side]) {
			return fmt.Errorf("tests: L2 and L3 depth disagree for %s", symbol)
		}

		for price, quantity := range aggregated[side] {
			if math.Abs(validator.books[symbol][side][price]-quantity) > 1e-8 {
				return fmt.Errorf("tests: L2 does not aggregate L3 for %s", symbol)
			}
		}
	}

	return nil
}
