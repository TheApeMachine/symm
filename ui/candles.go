package ui

import (
	"sync"
	"time"

	"github.com/theapemachine/symm/config"
)

/*
CandleBar is one server-side OHLC bucket for chart rendering.
*/
type CandleBar struct {
	Sec   int64   `json:"sec"`
	Open  float64 `json:"open"`
	High  float64 `json:"high"`
	Low   float64 `json:"low"`
	Close float64 `json:"close"`
}

type candleBucket struct {
	sec   int64
	open  float64
	high  float64
	low   float64
	close float64
}

/*
CandleAggregator maintains per-symbol OHLC buckets for UI-ready chart bars.
*/
type CandleAggregator struct {
	mu       sync.Mutex
	interval int64
	buckets  map[string]*candleBucket
}

/*
NewCandleAggregator creates a candle aggregator with config-driven bucket size.
*/
func NewCandleAggregator() *CandleAggregator {
	seconds := int64(config.System.CandleSeconds)

	if seconds <= 0 {
		seconds = 5
	}

	return &CandleAggregator{
		interval: seconds,
		buckets:  make(map[string]*candleBucket),
	}
}

/*
Update ingests one quote and returns the current bucket bar when valid.
*/
func (aggregator *CandleAggregator) Update(
	symbol string,
	last float64,
	at time.Time,
) (CandleBar, bool) {
	if aggregator == nil || symbol == "" || last <= 0 {
		return CandleBar{}, false
	}

	sec := at.Unix()

	if sec <= 0 {
		sec = time.Now().Unix()
	}

	bucketSec := (sec / aggregator.interval) * aggregator.interval

	aggregator.mu.Lock()
	defer aggregator.mu.Unlock()

	bucket := aggregator.buckets[symbol]

	if bucket == nil || bucket.sec != bucketSec {
		bucket = &candleBucket{
			sec:   bucketSec,
			open:  last,
			high:  last,
			low:   last,
			close: last,
		}
		aggregator.buckets[symbol] = bucket

		return bucket.toBar(), true
	}

	bucket.high = maxFloat(bucket.high, last)
	bucket.low = minFloat(bucket.low, last)
	bucket.close = last

	return bucket.toBar(), true
}

func (bucket *candleBucket) toBar() CandleBar {
	return CandleBar{
		Sec:   bucket.sec,
		Open:  bucket.open,
		High:  bucket.high,
		Low:   bucket.low,
		Close: bucket.close,
	}
}

func maxFloat(left, right float64) float64 {
	if right > left {
		return right
	}

	return left
}

func minFloat(left, right float64) float64 {
	if right < left {
		return right
	}

	return left
}
