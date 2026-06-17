package toxicity

import (
	"strconv"
	"time"

	krakenmarket "github.com/theapemachine/symm/kraken/market"
)

/*
IngestTrade feeds one public trade print into the default toxicity tracker.
*/
func IngestTrade(
	symbol string,
	pair krakenmarket.Pair,
	price float64,
	volume float64,
	at time.Time,
) {
	if symbol == "" || price <= 0 || volume <= 0 {
		return
	}

	tracker := defaultTracker.Load()
	tracker.ObserveTrade(symbol, pair, price, volume, at)
	tracker.ObserveLast(symbol, pair, price)
}

/*
IngestBook feeds one public book frame into the default toxicity tracker.
*/
func IngestBook(
	symbol string,
	pair krakenmarket.Pair,
	book *krakenmarket.BookUpdate,
	at time.Time,
) {
	if symbol == "" || book == nil {
		return
	}

	tracker := defaultTracker.Load()

	if book.Type == "snapshot" || book.Type == "" {
		tracker.ApplyBookFrame(symbol, pair, book, at)
	} else {
		tracker.ApplyBookDelta(symbol, pair, book, at)
	}

	mid := touchMid(book)

	if mid > 0 {
		tracker.ObserveMid(symbol, pair, mid)
	}
}

/*
ReplayBookPayload encodes book-quality features for tree replay measurement.
*/
func ReplayBookPayload(symbol string) ([]float64, bool) {
	tracker := defaultTracker.Load()
	features, ok := tracker.measureFeatures(symbol)

	if !ok || features.lastPrice <= 0 {
		return nil, false
	}

	snapshot := features.snapshot

	return []float64{
		snapshot.cancelBid,
		snapshot.fillBid,
		snapshot.cancelAsk,
		snapshot.fillAsk,
		snapshot.bidDepth,
		snapshot.askDepth,
		boolSample(snapshot.toxicNear),
		snapshot.toxicBluffStrength,
		features.threshold,
		features.churnGate,
		features.supportGate,
		features.vacuumStrengthCap,
		features.lastPrice,
	}, true
}

func touchMid(book *krakenmarket.BookUpdate) float64 {
	if book == nil || len(book.Bids) == 0 || len(book.Asks) == 0 {
		return 0
	}

	bid := book.Bids[0].Price
	ask := book.Asks[0].Price

	if bid <= 0 || ask <= bid {
		return 0
	}

	return (bid + ask) / 2
}

func boolSample(value bool) float64 {
	if value {
		return 1
	}

	return 0
}

/*
PairFromTick builds minimal pair metadata for tracker keying.
*/
func PairFromTick(symbol string, tickSize float64) krakenmarket.Pair {
	pair := krakenmarket.Pair{
		Wsname: symbol,
	}

	if tickSize > 0 {
		pair.TickSize = strconv.FormatFloat(tickSize, 'f', -1, 64)
	}

	return pair
}
