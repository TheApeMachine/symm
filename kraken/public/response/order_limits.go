package response

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/spf13/viper"
)

const (
	paperRateLimitExceeded = "EOrder:Rate limit exceeded"
	paperOpenLimitExceeded = "EOrder:Orders limit exceeded"
	paperUnknownOrder      = "EOrder:Unknown order"
)

type paperTradingLimits struct {
	mu        sync.Mutex
	enabled   bool
	decay     float64
	threshold float64
	openLimit int
	pairs     map[string]*paperPairLimits
	index     map[string]paperOrderIndex
	now       func() time.Time
}

type paperPairLimits struct {
	counter     float64
	lastUpdated time.Time
	open        map[string]paperOpenOrder
}

type paperOpenOrder struct {
	orderID   string
	clOrdID   string
	symbol    string
	createdAt time.Time
	amendedAt time.Time
}

type paperOrderIndex struct {
	symbol  string
	orderID string
}

func newPaperTradingLimits() *paperTradingLimits {
	decay, threshold, openLimit := paperTradingLimitTier()

	if override := viper.GetFloat64("trading.paper.rate_limits.decay_per_second"); override > 0 {
		decay = override
	}
	if override := viper.GetFloat64("trading.paper.rate_limits.threshold"); override > 0 {
		threshold = override
	}
	if override := viper.GetInt("trading.paper.rate_limits.open_orders_per_pair"); override > 0 {
		openLimit = override
	}

	enabled := true
	if viper.IsSet("trading.paper.rate_limits.enabled") {
		enabled = viper.GetBool("trading.paper.rate_limits.enabled")
	}

	return &paperTradingLimits{
		enabled:   enabled,
		decay:     decay,
		threshold: threshold,
		openLimit: openLimit,
		pairs:     make(map[string]*paperPairLimits),
		index:     make(map[string]paperOrderIndex),
		now:       time.Now,
	}
}

func paperTradingLimitTier() (float64, float64, int) {
	switch strings.ToLower(strings.TrimSpace(viper.GetString("trading.paper.rate_limits.tier"))) {
	case "intermediate":
		return 2.34, 125, 80
	case "pro":
		return 3.75, 180, 225
	default:
		return 1, 60, 60
	}
}

func (limits *paperTradingLimits) Add(symbol string, resting bool, now time.Time) error {
	if limits == nil || !limits.enabled {
		return nil
	}

	limits.mu.Lock()
	defer limits.mu.Unlock()

	state := limits.pairState(symbol, now)
	limits.decayCounter(state, now)
	state.counter++

	if state.counter > limits.threshold {
		return fmt.Errorf(paperRateLimitExceeded)
	}
	if resting && limits.openLimit > 0 && len(state.open) >= limits.openLimit {
		return fmt.Errorf(paperOpenLimitExceeded)
	}

	return nil
}

func (limits *paperTradingLimits) Open(
	symbol string,
	orderID string,
	clOrdID string,
	now time.Time,
) {
	if limits == nil || !limits.enabled || symbol == "" || orderID == "" {
		return
	}

	limits.mu.Lock()
	defer limits.mu.Unlock()

	state := limits.pairState(symbol, now)
	open := paperOpenOrder{
		orderID:   orderID,
		clOrdID:   clOrdID,
		symbol:    normalizePaperPair(symbol),
		createdAt: now,
	}
	state.open[orderID] = open
	limits.index[orderID] = paperOrderIndex{symbol: open.symbol, orderID: orderID}
	if clOrdID != "" {
		limits.index[clOrdID] = paperOrderIndex{symbol: open.symbol, orderID: orderID}
	}
}

func (limits *paperTradingLimits) Cancel(
	symbol string,
	identifier string,
	now time.Time,
) (paperOpenOrder, bool, error) {
	if limits == nil || !limits.enabled {
		return paperOpenOrder{}, false, nil
	}

	limits.mu.Lock()
	defer limits.mu.Unlock()

	index, ok := limits.resolveOrderIndex(symbol, identifier)
	if !ok {
		return paperOpenOrder{}, false, fmt.Errorf(paperUnknownOrder)
	}

	state := limits.pairState(index.symbol, now)
	limits.decayCounter(state, now)

	open, ok := state.open[index.orderID]
	if !ok {
		return paperOpenOrder{}, false, fmt.Errorf(paperUnknownOrder)
	}

	state.counter += cancelRatePenalty(open, now)
	if state.counter > limits.threshold {
		return open, true, fmt.Errorf(paperRateLimitExceeded)
	}

	delete(state.open, index.orderID)
	delete(limits.index, open.orderID)
	if open.clOrdID != "" {
		delete(limits.index, open.clOrdID)
	}

	return open, true, nil
}

func (limits *paperTradingLimits) Amend(
	method string,
	symbol string,
	identifier string,
	now time.Time,
) (paperOpenOrder, bool, error) {
	if limits == nil || !limits.enabled {
		return paperOpenOrder{}, false, nil
	}

	limits.mu.Lock()
	defer limits.mu.Unlock()

	index, ok := limits.resolveOrderIndex(symbol, identifier)
	if !ok {
		symbol = normalizePaperPair(symbol)
		if symbol == "" {
			return paperOpenOrder{}, false, fmt.Errorf(paperUnknownOrder)
		}

		state := limits.pairState(symbol, now)
		limits.decayCounter(state, now)
		state.counter++
		if state.counter > limits.threshold {
			return paperOpenOrder{}, false, fmt.Errorf(paperRateLimitExceeded)
		}

		return paperOpenOrder{}, false, fmt.Errorf(paperUnknownOrder)
	}

	state := limits.pairState(index.symbol, now)
	limits.decayCounter(state, now)

	open, ok := state.open[index.orderID]
	if !ok {
		return paperOpenOrder{}, false, fmt.Errorf(paperUnknownOrder)
	}

	state.counter += 1 + amendRatePenalty(method, open, now)
	if state.counter > limits.threshold {
		return open, true, fmt.Errorf(paperRateLimitExceeded)
	}

	open.amendedAt = now
	state.open[index.orderID] = open

	return open, true, nil
}

func (limits *paperTradingLimits) SymbolForOrder(identifier string) string {
	if limits == nil || identifier == "" {
		return ""
	}

	limits.mu.Lock()
	defer limits.mu.Unlock()

	index, ok := limits.index[identifier]
	if !ok {
		return ""
	}

	return index.symbol
}

func (limits *paperTradingLimits) pairState(symbol string, now time.Time) *paperPairLimits {
	symbol = normalizePaperPair(symbol)
	state := limits.pairs[symbol]
	if state != nil {
		return state
	}

	state = &paperPairLimits{
		lastUpdated: now,
		open:        make(map[string]paperOpenOrder),
	}
	limits.pairs[symbol] = state
	return state
}

func (limits *paperTradingLimits) decayCounter(state *paperPairLimits, now time.Time) {
	if state == nil {
		return
	}
	if now.IsZero() {
		now = limits.now()
	}
	if state.lastUpdated.IsZero() {
		state.lastUpdated = now
		return
	}

	elapsed := now.Sub(state.lastUpdated).Seconds()
	if elapsed > 0 && limits.decay > 0 {
		state.counter -= elapsed * limits.decay
		if state.counter < 0 {
			state.counter = 0
		}
	}
	state.lastUpdated = now
}

func (limits *paperTradingLimits) resolveOrderIndex(
	symbol string,
	identifier string,
) (paperOrderIndex, bool) {
	identifier = strings.TrimSpace(identifier)
	if identifier != "" {
		index, ok := limits.index[identifier]
		if ok {
			return index, true
		}
	}

	symbol = normalizePaperPair(symbol)
	if symbol == "" {
		return paperOrderIndex{}, false
	}
	if identifier == "" {
		return paperOrderIndex{}, false
	}

	state := limits.pairs[symbol]
	if state == nil {
		return paperOrderIndex{}, false
	}
	if _, ok := state.open[identifier]; !ok {
		return paperOrderIndex{}, false
	}

	return paperOrderIndex{symbol: symbol, orderID: identifier}, true
}

func cancelRatePenalty(open paperOpenOrder, now time.Time) float64 {
	anchor := open.createdAt
	if !open.amendedAt.IsZero() && open.amendedAt.After(anchor) {
		anchor = open.amendedAt
	}

	age := now.Sub(anchor)
	switch {
	case age < 5*time.Second:
		return 8
	case age < 10*time.Second:
		return 6
	case age < 15*time.Second:
		return 5
	case age < 45*time.Second:
		return 4
	case age < 90*time.Second:
		return 2
	case age < 300*time.Second:
		return 1
	default:
		return 0
	}
}

func amendRatePenalty(method string, open paperOpenOrder, now time.Time) float64 {
	anchor := open.createdAt
	if !open.amendedAt.IsZero() && open.amendedAt.After(anchor) {
		anchor = open.amendedAt
	}

	age := now.Sub(anchor)
	switch strings.ToLower(strings.TrimSpace(method)) {
	case "edit_order":
		switch {
		case age < 5*time.Second:
			return 6
		case age < 10*time.Second:
			return 5
		case age < 15*time.Second:
			return 4
		case age < 45*time.Second:
			return 2
		case age < 90*time.Second:
			return 1
		default:
			return 0
		}
	default:
		switch {
		case age < 5*time.Second:
			return 3
		case age < 10*time.Second:
			return 2
		case age < 15*time.Second:
			return 1
		default:
			return 0
		}
	}
}

func normalizePaperPair(symbol string) string {
	return strings.ToUpper(strings.TrimSpace(symbol))
}
