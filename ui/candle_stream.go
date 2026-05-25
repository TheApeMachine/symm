package ui

import (
	"fmt"
	"math"
	"sync"
	"time"

	"github.com/theapemachine/qpool"
)

/*
CandleBar is one source-side OHLC bucket emitted to the dashboard.
*/
type CandleBar struct {
	Symbol string
	Sec    int64
	Open   float64
	High   float64
	Low    float64
	Close  float64
	Volume float64
}

/*
CandleStream builds per-symbol OHLCV bars from market prices before UI broadcast.
*/
type CandleStream struct {
	mu              sync.Mutex
	intervalSeconds int64
	bySymbol        map[string]CandleBar
}

/*
NewCandleStream creates a source-side chart candle accumulator.
*/
func NewCandleStream(interval time.Duration) (*CandleStream, error) {
	if interval <= 0 {
		return nil, fmt.Errorf("candle stream requires positive interval")
	}

	if interval%time.Second != 0 {
		return nil, fmt.Errorf("candle stream interval must be second-aligned")
	}

	return &CandleStream{
		intervalSeconds: int64(interval / time.Second),
		bySymbol:        make(map[string]CandleBar),
	}, nil
}

/*
Observe records one timestamped price and returns the complete current bar.
*/
func (candleStream *CandleStream) Observe(
	symbol string,
	price float64,
	at time.Time,
) (CandleBar, error) {
	return candleStream.ObserveTrade(symbol, price, 0, at)
}

/*
ObserveTrade records one timestamped trade and returns the complete current bar.
*/
func (candleStream *CandleStream) ObserveTrade(
	symbol string,
	price float64,
	volume float64,
	at time.Time,
) (CandleBar, error) {
	if candleStream == nil {
		return CandleBar{}, fmt.Errorf("candle stream is nil")
	}

	if symbol == "" {
		return CandleBar{}, fmt.Errorf("candle stream requires symbol")
	}

	if price <= 0 || math.IsNaN(price) || math.IsInf(price, 0) {
		return CandleBar{}, fmt.Errorf("candle stream invalid price %f for %s", price, symbol)
	}

	if volume < 0 || math.IsNaN(volume) || math.IsInf(volume, 0) {
		return CandleBar{}, fmt.Errorf("candle stream invalid volume %f for %s", volume, symbol)
	}

	if at.IsZero() {
		return CandleBar{}, fmt.Errorf("candle stream requires timestamp for %s", symbol)
	}

	bucketSec := at.Unix() / candleStream.intervalSeconds * candleStream.intervalSeconds

	candleStream.mu.Lock()
	defer candleStream.mu.Unlock()

	current, ok := candleStream.bySymbol[symbol]

	if ok && bucketSec < current.Sec {
		return CandleBar{}, fmt.Errorf(
			"candle stream out-of-order tick for %s: %d < %d",
			symbol,
			bucketSec,
			current.Sec,
		)
	}

	if ok && bucketSec == current.Sec {
		current.High = math.Max(current.High, price)
		current.Low = math.Min(current.Low, price)
		current.Close = price
		current.Volume += volume
		candleStream.bySymbol[symbol] = current

		return current, nil
	}

	current = CandleBar{
		Symbol: symbol,
		Sec:    bucketSec,
		Open:   price,
		High:   price,
		Low:    price,
		Close:  price,
		Volume: volume,
	}
	candleStream.bySymbol[symbol] = current

	return current, nil
}

/*
ObserveTicker records one Kraken ticker row and returns the current candle bar.
*/
func (candleStream *CandleStream) ObserveTicker(
	symbol string,
	price float64,
	timestamp string,
) (CandleBar, error) {
	at, err := time.Parse(time.RFC3339Nano, timestamp)

	if err != nil {
		return CandleBar{}, fmt.Errorf("parse ticker timestamp for %s: %w", symbol, err)
	}

	return candleStream.Observe(symbol, price, at)
}

/*
PublishTicker emits one candle_bar payload to the shared ui broadcast group.
*/
func (candleStream *CandleStream) PublishTicker(
	uiGroup *qpool.BroadcastGroup,
	symbol string,
	price float64,
	timestamp string,
) error {
	if uiGroup == nil {
		return fmt.Errorf("candle stream publish requires ui broadcast group")
	}

	candleBar, err := candleStream.ObserveTicker(symbol, price, timestamp)

	if err != nil {
		return err
	}

	Publish(uiGroup, "candle_bar", candleBar.Payload())

	return nil
}

/*
Payload converts a candle bar into the websocket event body.
*/
func (candleBar CandleBar) Payload() map[string]any {
	return map[string]any{
		"symbol": candleBar.Symbol,
		"sec":    candleBar.Sec,
		"open":   candleBar.Open,
		"high":   candleBar.High,
		"low":    candleBar.Low,
		"close":  candleBar.Close,
		"volume": candleBar.Volume,
	}
}
