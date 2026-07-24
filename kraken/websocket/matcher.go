package websocket

import (
	"fmt"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/theapemachine/datura"
)

/*
Matcher is an in-process paper exchange. It owns wallet balances and last marks,
fills market and limit orders against those marks, and never shells out to an
external CLI. Timing waits belong to the injected Simulator clock.
*/
type Matcher struct {
	mu       sync.Mutex
	balances map[string]float64
	marks    map[string]float64
	feeRate  float64
	nextID   atomic.Uint64
	clock    Clock
}

/*
NewMatcher constructs a paper matcher with a quote seed balance and fee rate.
*/
func NewMatcher(clock Clock, quote string, seed float64, feeRate float64) *Matcher {
	if clock == nil {
		clock = WallClock{}
	}

	if quote == "" {
		quote = "USD"
	}

	if feeRate <= 0 {
		feeRate = 0.0026
	}

	matcher := &Matcher{
		balances: map[string]float64{quote: seed},
		marks:    map[string]float64{},
		feeRate:  feeRate,
		clock:    clock,
	}

	return matcher
}

/*
SetMark records the last tradeable mid for a symbol.
*/
func (matcher *Matcher) SetMark(symbol string, price float64) {
	matcher.mu.Lock()
	defer matcher.mu.Unlock()

	matcher.marks[symbol] = price
}

/*
SeedBalance sets an absolute wallet total for an asset.
*/
func (matcher *Matcher) SeedBalance(asset string, total float64) {
	matcher.mu.Lock()
	defer matcher.mu.Unlock()

	matcher.balances[asset] = total
}

/*
Balances returns a NewBalanceFromMap-compatible wallet dump.
*/
func (matcher *Matcher) Balances() datura.Map[any] {
	matcher.mu.Lock()
	defer matcher.mu.Unlock()

	balances := map[string]any{}

	for asset, total := range matcher.balances {
		if total == 0 {
			continue
		}

		balances[asset] = map[string]any{
			"available": total,
			"reserved":  0.0,
			"total":     total,
		}
	}

	return datura.Map[any]{
		"balances": balances,
		"mode":     "paper",
	}
}

/*
Fill executes one order against the current mark and updates balances.
*/
func (matcher *Matcher) Fill(
	side string, symbol string, quantity float64, limit float64,
) (datura.Map[any], error) {
	matcher.mu.Lock()
	defer matcher.mu.Unlock()

	price := limit

	if price <= 0 {
		mark, ok := matcher.marks[symbol]

		if !ok || mark <= 0 {
			return nil, fmt.Errorf("paper: no mark for %s", symbol)
		}

		price = mark
	}

	cost := quantity * price
	fee := cost * matcher.feeRate
	base, quote := splitSymbol(symbol)
	orderID := matcher.id("ORDER")
	tradeID := matcher.id("TRADE")

	switch strings.ToLower(side) {
	case "buy":
		if matcher.balances[quote] < cost+fee {
			return nil, fmt.Errorf("paper: insufficient %s", quote)
		}

		matcher.balances[quote] -= cost + fee
		matcher.balances[base] += quantity
	case "sell":
		if matcher.balances[base] < quantity {
			return nil, fmt.Errorf("paper: insufficient %s", base)
		}

		matcher.balances[base] -= quantity
		matcher.balances[quote] += cost - fee
	default:
		return nil, fmt.Errorf("paper: unknown side %s", side)
	}

	matcher.marks[symbol] = price

	return datura.Map[any]{
		"action":   "market_order_filled",
		"order_id": orderID,
		"trade_id": tradeID,
		"pair":     symbol,
		"side":     side,
		"volume":   quantity,
		"price":    price,
		"cost":     cost,
		"fee":      fee,
		"status":   "filled",
		"time":     matcher.clock.Now().UTC().Format(time.RFC3339),
	}, nil
}

func (matcher *Matcher) id(prefix string) string {
	return fmt.Sprintf("PAPER-%s-%d", prefix, matcher.nextID.Add(1))
}

func splitSymbol(symbol string) (base string, quote string) {
	if left, right, ok := strings.Cut(symbol, "/"); ok {
		return left, right
	}

	if len(symbol) > 3 {
		return symbol[:len(symbol)-3], symbol[len(symbol)-3:]
	}

	return symbol, "USD"
}

/*
ParseQuantity converts a wire quantity number into float64 for matching.
*/
func ParseQuantity(raw string) (float64, error) {
	return strconv.ParseFloat(raw, 64)
}
