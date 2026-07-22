package tests

import (
	"fmt"
	"math"
	"time"
)

/*
tickerState retains independent trade facts used to verify ticker projection.
*/
type tickerState struct {
	volume   float64
	notional float64
	open     float64
	high     float64
	low      float64
	last     float64
	tradeID  int64
	at       time.Time
}

/*
validateTimes rejects missing, regressed, or cross-feed event clocks.
*/
func (validator *Validator) validateTimes(
	tickers []wireTicker,
	trades []wireTrade,
	books []wireBook,
	level3 []wireLevel3,
) error {
	frameTimes := map[string]time.Time{}
	record := func(symbol string, at time.Time) error {
		if symbol == "" || at.IsZero() {
			return fmt.Errorf("tests: simulated event identity required")
		}

		if frameAt, exists := frameTimes[symbol]; exists && !frameAt.Equal(at) {
			return fmt.Errorf("tests: simulated feeds disagree on time for %s", symbol)
		}

		if at.Before(validator.observed[symbol]) {
			return fmt.Errorf("tests: simulated event time regressed for %s", symbol)
		}

		frameTimes[symbol] = at
		validator.observed[symbol] = at
		return nil
	}

	for _, row := range tickers {
		if err := record(row.Symbol, row.Timestamp); err != nil {
			return err
		}
	}

	for _, row := range trades {
		if err := record(row.Symbol, row.Timestamp); err != nil {
			return err
		}
	}

	for _, row := range books {
		if err := record(row.Symbol, row.Timestamp); err != nil {
			return err
		}
	}

	for _, row := range level3 {
		if err := record(row.Symbol, row.Timestamp); err != nil {
			return err
		}
	}

	return nil
}

/*
validateTicker reconstructs cumulative trade statistics independently.
*/
func (validator *Validator) validateTicker(
	ticker wireTicker,
	trades []wireTrade,
) error {
	state := validator.ticker[ticker.Symbol]
	high, err := ticker.High.Float64()

	if err != nil {
		return fmt.Errorf("tests: invalid ticker high for %s: %w", ticker.Symbol, err)
	}

	low, err := ticker.Low.Float64()

	if err != nil {
		return fmt.Errorf("tests: invalid ticker low for %s: %w", ticker.Symbol, err)
	}

	last, err := ticker.Last.Float64()

	if err != nil {
		return fmt.Errorf("tests: invalid ticker last for %s: %w", ticker.Symbol, err)
	}

	if len(trades) == 0 {
		return state.validateQuiet(ticker, high, low, last)
	}

	state, err = state.measure(ticker.Symbol, trades)

	if err != nil {
		return err
	}

	if err := state.validate(ticker, high, low); err != nil {
		return err
	}

	validator.ticker[ticker.Symbol] = state
	return nil
}

/*
measure applies one ordered execution batch to retained ticker statistics.
*/
func (state tickerState) measure(
	symbol string,
	trades []wireTrade,
) (tickerState, error) {
	for _, trade := range trades {
		tradePrice, err := trade.Price.Float64()

		if err != nil || tradePrice <= 0 || trade.Qty <= 0 {
			return state, fmt.Errorf("tests: invalid trade for %s", symbol)
		}

		if trade.TradeID <= state.tradeID ||
			!state.at.IsZero() && trade.Timestamp.Before(state.at) {
			return state, fmt.Errorf("tests: trade sequence is not monotonic for %s", symbol)
		}

		state.volume += trade.Qty
		state.notional += tradePrice * trade.Qty

		if state.open == 0 {
			state.open = tradePrice
		}

		state.high = max(state.high, tradePrice)

		if state.low == 0 {
			state.low = tradePrice
		}

		state.low = min(state.low, tradePrice)
		state.tradeID = trade.TradeID
		state.at = trade.Timestamp
		state.last = tradePrice
	}

	return state, nil
}

/*
validateQuiet rejects cumulative ticker changes that lack a matching trade.
*/
func (state tickerState) validateQuiet(
	ticker wireTicker,
	high float64,
	low float64,
	last float64,
) error {
	if state.open <= 0 || state.volume <= 0 {
		return fmt.Errorf("tests: ticker has no trade history for %s", ticker.Symbol)
	}

	if last != state.last {
		return fmt.Errorf("tests: ticker changed without a trade for %s", ticker.Symbol)
	}

	return state.validate(ticker, high, low)
}

/*
validate proves every cumulative ticker field from retained trades.
*/
func (state tickerState) validate(
	ticker wireTicker,
	high float64,
	low float64,
) error {
	if math.Abs(ticker.Volume-state.volume) > 1e-8 ||
		math.Abs(ticker.VWAP-state.notional/state.volume) > 1e-8 ||
		high != state.high || low != state.low {
		return fmt.Errorf("tests: ticker statistics do not reconcile for %s", ticker.Symbol)
	}

	change := state.last - state.open

	if math.Abs(ticker.Change-change) > 1e-8 ||
		math.Abs(ticker.ChangePct-change/state.open*100) > 1e-8 {
		return fmt.Errorf("tests: ticker change does not reconcile for %s", ticker.Symbol)
	}

	return nil
}
