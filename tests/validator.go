package tests

import (
	"bytes"
	"encoding/json"
	"fmt"
	"hash/crc32"
	"math"
	"sort"
	"strconv"
	"strings"
)

/*
Validator reconstructs the emitted venue state and rejects cross-feed conflicts
before any fixture payload reaches the production connection boundary.
*/
type Validator struct {
	books  map[string]map[string]map[float64]float64
	orders map[string]map[string]map[string]orderState
	ticker map[string]tickerState
}

/*
NewValidator creates an empty invariant gate for one deterministic venue.
*/
func NewValidator() *Validator {
	return &Validator{
		books:  map[string]map[string]map[float64]float64{},
		orders: map[string]map[string]map[string]orderState{},
		ticker: map[string]tickerState{},
	}
}

/*
Validate applies one coherent frame set and verifies its economic invariants.
*/
func (validator *Validator) Validate(payloads frameSet) error {
	var ticker wireFrame[wireTicker]
	var trades wireFrame[wireTrade]
	var book wireFrame[wireBook]
	var level3 wireFrame[wireLevel3]

	for _, input := range []struct {
		payload []byte
		frame   any
	}{
		{payloads.ticker, &ticker},
		{payloads.trade, &trades},
		{payloads.book, &book},
		{payloads.level3, &level3},
	} {
		decoder := json.NewDecoder(bytes.NewReader(input.payload))
		decoder.UseNumber()

		if err := decoder.Decode(input.frame); err != nil {
			return fmt.Errorf("tests: decode simulated frame: %w", err)
		}
	}

	if len(ticker.Data) != len(book.Data) || len(ticker.Data) != len(level3.Data) {
		return fmt.Errorf("tests: simulated frame symbol counts differ")
	}

	tradesBySymbol := make(map[string]*wireTrade, len(trades.Data))

	for index := range trades.Data {
		trade := &trades.Data[index]
		tradesBySymbol[trade.Symbol] = trade
	}

	for index := range ticker.Data {
		if err := validator.applyLevel3(level3.Type, level3.Data[index]); err != nil {
			return err
		}

		if err := validator.applyBook(book.Type, book.Data[index]); err != nil {
			return err
		}

		if err := validator.validateSymbol(
			ticker.Data[index], tradesBySymbol[ticker.Data[index].Symbol],
			book.Data[index], level3.Data[index],
		); err != nil {
			return err
		}
	}

	return nil
}

func (validator *Validator) applyLevel3(frameType string, row wireLevel3) error {
	if frameType == "snapshot" || validator.orders[row.Symbol] == nil {
		validator.orders[row.Symbol] = map[string]map[string]orderState{
			"bids": {},
			"asks": {},
		}
	}

	for side, orders := range map[string][]wireOrder{"bids": row.Bids, "asks": row.Asks} {
		for _, order := range orders {
			current := validator.orders[row.Symbol][side]
			_, exists := current[order.OrderID]

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

			current[order.OrderID] = orderState{
				price: order.Price.String(),
				qty:   order.Qty.String(),
			}
		}
	}

	if validator.level3Checksum(row.Symbol) != row.Checksum {
		return fmt.Errorf("tests: invalid L3 checksum for %s", row.Symbol)
	}

	return nil
}

func (validator *Validator) applyBook(frameType string, row wireBook) error {
	if frameType == "snapshot" || validator.books[row.Symbol] == nil {
		validator.books[row.Symbol] = map[string]map[float64]float64{
			"bids": {},
			"asks": {},
		}
	}

	for side, levels := range map[string][]wireLevel{"bids": row.Bids, "asks": row.Asks} {
		for _, level := range levels {
			price, _ := level.Price.Float64()
			quantity, _ := level.Qty.Float64()

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

func (validator *Validator) validateSymbol(
	ticker wireTicker,
	trade *wireTrade,
	book wireBook,
	level3 wireLevel3,
) error {
	if ticker.Symbol != book.Symbol || ticker.Symbol != level3.Symbol ||
		trade != nil && ticker.Symbol != trade.Symbol {
		return fmt.Errorf("tests: simulated frame symbols differ")
	}

	symbol := ticker.Symbol
	bid, bidQty := validator.touch(symbol, "bids", true)
	ask, askQty := validator.touch(symbol, "asks", false)
	tickerBid, _ := ticker.Bid.Float64()
	tickerAsk, _ := ticker.Ask.Float64()
	tickerLast, _ := ticker.Last.Float64()

	if bid >= ask || math.Abs(tickerBid-bid) > 1e-8 ||
		math.Abs(tickerAsk-ask) > 1e-8 ||
		math.Abs(ticker.BidQty-bidQty) > 1e-8 ||
		math.Abs(ticker.AskQty-askQty) > 1e-8 {
		return fmt.Errorf("tests: ticker and book touch disagree for %s", symbol)
	}

	if err := validator.validateAggregates(symbol); err != nil {
		return err
	}

	if trade == nil {
		return validator.validateTicker(ticker, nil, tickerLast)
	}

	tradePrice, _ := trade.Price.Float64()

	if trade.Side == "buy" && math.Abs(tradePrice-ask) > 1e-8 ||
		trade.Side == "sell" && math.Abs(tradePrice-bid) > 1e-8 {
		return fmt.Errorf("tests: %s trade did not execute at touch for %s", trade.Side, symbol)
	}

	if math.Abs(tickerLast-tradePrice) > 1e-8 {
		return fmt.Errorf("tests: ticker last and trade price disagree for %s", symbol)
	}

	return validator.validateTicker(ticker, trade, tradePrice)
}

func (validator *Validator) validateAggregates(symbol string) error {
	aggregated := map[string]map[float64]float64{"bids": {}, "asks": {}}

	for side, orders := range validator.orders[symbol] {
		for _, order := range orders {
			price, _ := strconv.ParseFloat(order.price, 64)
			quantity, _ := strconv.ParseFloat(order.qty, 64)
			aggregated[side][price] += quantity
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

func (validator *Validator) validateTicker(
	ticker wireTicker,
	trade *wireTrade,
	tradePrice float64,
) error {
	state := validator.ticker[ticker.Symbol]
	high, _ := ticker.High.Float64()
	low, _ := ticker.Low.Float64()

	if trade == nil {
		if math.Abs(ticker.Volume-state.volume) > 1e-8 ||
			math.Abs(ticker.VWAP-state.notional/state.volume) > 1e-8 ||
			high != state.high || low != state.low {
			return fmt.Errorf("tests: ticker changed without a trade for %s", ticker.Symbol)
		}

		return nil
	}

	if trade.TradeID <= state.tradeID || !state.at.IsZero() && !trade.Timestamp.After(state.at) {
		return fmt.Errorf("tests: trade sequence is not monotonic for %s", ticker.Symbol)
	}

	state.volume += trade.Qty
	state.notional += tradePrice * trade.Qty

	if state.high == 0 || tradePrice > state.high {
		state.high = tradePrice
	}

	if state.low == 0 || tradePrice < state.low {
		state.low = tradePrice
	}

	if math.Abs(ticker.Volume-state.volume) > 1e-8 ||
		math.Abs(ticker.VWAP-state.notional/state.volume) > 1e-8 ||
		high != state.high || low != state.low {
		return fmt.Errorf("tests: ticker statistics do not reconcile for %s", ticker.Symbol)
	}

	state.tradeID = trade.TradeID
	state.at = trade.Timestamp
	validator.ticker[ticker.Symbol] = state
	return nil
}

func (validator *Validator) touch(
	symbol string,
	side string,
	highest bool,
) (float64, float64) {
	price := 0.0

	for candidate := range validator.books[symbol][side] {
		if price == 0 || highest && candidate > price || !highest && candidate < price {
			price = candidate
		}
	}

	return price, validator.books[symbol][side][price]
}

func (validator *Validator) level3Checksum(symbol string) uint32 {
	checksum := uint32(0)

	for _, side := range []string{"asks", "bids"} {
		orders := make([]orderState, 0, len(validator.orders[symbol][side]))

		for _, order := range validator.orders[symbol][side] {
			orders = append(orders, order)
		}

		sort.Slice(orders, func(left, right int) bool {
			leftPrice, _ := strconv.ParseFloat(orders[left].price, 64)
			rightPrice, _ := strconv.ParseFloat(orders[right].price, 64)

			if side == "bids" {
				return leftPrice > rightPrice
			}

			return leftPrice < rightPrice
		})

		for _, order := range orders {
			for _, value := range []string{order.price, order.qty} {
				normalized := strings.TrimLeft(strings.ReplaceAll(value, ".", ""), "0")
				checksum = crc32.Update(checksum, crc32.IEEETable, []byte(normalized))
			}
		}
	}

	return checksum
}

/*
bookChecksum derives Kraken's CRC from the reconstructed L2 state.
*/
func (validator *Validator) bookChecksum(symbol string) uint32 {
	checksum := uint32(0)

	for _, side := range []string{"asks", "bids"} {
		prices := make([]float64, 0, len(validator.books[symbol][side]))

		for price := range validator.books[symbol][side] {
			prices = append(prices, price)
		}

		sort.Float64s(prices)

		if side == "bids" {
			sort.Sort(sort.Reverse(sort.Float64Slice(prices)))
		}

		for _, price := range prices {
			for _, value := range []float64{price, validator.books[symbol][side][price]} {
				text := strconv.FormatFloat(value, 'f', -1, 64)
				normalized := strings.TrimLeft(strings.ReplaceAll(text, ".", ""), "0")
				checksum = crc32.Update(checksum, crc32.IEEETable, []byte(normalized))
			}
		}
	}

	return checksum
}
