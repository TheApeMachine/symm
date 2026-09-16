package tables

import (
	"fmt"
	"math"
	"unsafe"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/nomagique"
	"github.com/theapemachine/symm/nomagique/adaptive"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/statistic"
	"github.com/theapemachine/symm/nomagique/transport"
)

type symbolTracker struct {
	window                *nomagique.Number
	bid, ask, entry, cost *decimal.Decimal
	high, low             *decimal.Decimal
	highTick, lowTick     int64
	anchor, observations  int64
}

/*
StreamingDetector resolves intervals when a volume-weighted price baseline
changes. Quotes only update valuation; positive base-volume trades advance the
baseline. Every anchor is an observed sequence. It retains sufficient statistics
and one open interval per symbol, never tape fragments or a simulated wallet.
Returns grade a quoted base unit after the venue's actual fees. They describe
quote returns, not a claim that arbitrary order sizes could have filled.
The catalog drain is its sole writer.
*/
type StreamingDetector struct {
	epoch    int64
	price    *broker.Price
	trackers map[string]*symbolTracker
}

func NewStreamingDetector(epoch int64, price *broker.Price) *StreamingDetector {
	return &StreamingDetector{epoch: epoch, price: price, trackers: make(map[string]*symbolTracker)}
}

func (detector *StreamingDetector) Anchor(symbol string) int64 {
	if tracker := detector.trackers[symbol]; tracker != nil {
		return tracker.anchor
	}

	return 0
}

func (detector *StreamingDetector) Process(measurement *data.Measurement[float64]) (*ExcursionRecord, error) {
	if measurement.Metadata["venue"] != "true" || measurement.Metadata["volume-unit"] != "base" || measurement.Err != nil {
		return nil, nil
	}

	tracker := detector.trackers[measurement.Label]

	if tracker == nil {
		tracker = &symbolTracker{window: nomagique.NewNumber(adaptive.NewWeightedWindow())}
		detector.trackers[measurement.Label] = tracker
	}

	switch measurement.Provenance["channel"] {
	case "ticker":
		bid, ask := measurement.Metrics["bid"].Exact, measurement.Metrics["ask"].Exact

		if bid == nil || ask == nil || bid.Sign() <= 0 || ask.Cmp(bid) < 0 {
			return nil, errnie.Error(errnie.Err(errnie.Validation, "detector: exact uncrossed quote required", nil))
		}

		tracker.bid, tracker.ask = bid, ask
		return nil, nil
	case "trade":
		return detector.advance(tracker, measurement)
	}

	return nil, nil
}

func (detector *StreamingDetector) advance(tracker *symbolTracker, measurement *data.Measurement[float64]) (*ExcursionRecord, error) {
	quantity, price := measurement.Metrics["qty"].Exact, measurement.Metrics["price"].Exact

	if quantity == nil || price == nil || quantity.Sign() <= 0 || price.Sign() <= 0 {
		return nil, errnie.Error(errnie.Err(errnie.Validation, "detector: positive exact trade price and base volume required", nil))
	}

	// No quoted entry or liquidation value exists before both sides arrive.
	if tracker.bid == nil || tracker.ask == nil {
		return nil, nil
	}

	item := statistic.Weighted{Value: math.Log(price.Float64()), Weight: quantity.Float64()}
	var reading adaptive.WindowReading

	for pointer := range tracker.window.Next(transport.NewOne(unsafe.Pointer(&item)).Next(nil)) {
		reading = *(*adaptive.WindowReading)(pointer)
	}

	if err := tracker.window.Error(); err != nil {
		return nil, errnie.Error(err)
	}

	if tracker.high == nil || price.Cmp(tracker.high) > 0 {
		tracker.high, tracker.highTick = price, measurement.SeqIdx
	}
	if tracker.low == nil || price.Cmp(tracker.low) < 0 {
		tracker.low, tracker.lowTick = price, measurement.SeqIdx
	}

	tracker.observations++

	if tracker.anchor != 0 && reading.ShedRatio == 1 {
		return nil, nil
	}

	if detector.price == nil || detector.price.FeeIfAvailable(measurement.Label) == nil {
		return nil, errnie.Error(errnie.Err(errnie.Validation, "detector: authoritative fee required for "+measurement.Label, nil))
	}

	cost := detector.price.WithFee(measurement.Label, tracker.ask, broker.BUY)
	var record *ExcursionRecord

	if tracker.anchor != 0 {
		proceeds := detector.price.WithFee(measurement.Label, tracker.bid, broker.SELL)
		profit := proceeds.Sub(tracker.cost)
		direction := "flat"

		if tracker.bid.Cmp(tracker.entry) > 0 {
			direction = "upward"
		}

		if tracker.bid.Cmp(tracker.entry) < 0 {
			direction = "downward"
		}

		extreme, tick := tracker.high, tracker.highTick
		if direction == "downward" {
			extreme, tick = tracker.low, tracker.lowTick
		}

		record = &ExcursionRecord{
			Epoch: detector.epoch, ID: fmt.Sprintf("%d:%s:%d", detector.epoch, measurement.Label, tracker.anchor),
			Symbol: measurement.Label, Direction: direction, ClearsFriction: profit.Sign() > 0,
			AnchorTick: tracker.anchor, PrecursorStartTick: tracker.anchor,
			ExitTick: measurement.SeqIdx, PostEndTick: measurement.SeqIdx,
			EntryPrice: tracker.entry.Float64(), ExitPrice: tracker.bid.Float64(),
			PositionSize: tracker.cost.Float64(), Profit: profit.Float64(),
			ProfitFraction: profit.Div(tracker.cost).Float64(),
			Fee:            tracker.cost.Sub(tracker.entry).Add(tracker.bid.Sub(proceeds)).Float64(),
			ExtremumTick:   tick, ExtremumPrice: extreme.Float64(),
			GrossExcursion:   extreme.Sub(tracker.entry).Div(tracker.entry).Float64(),
			ObservationCount: tracker.observations, Status: "baseline_shift",
		}
	}

	tracker.anchor, tracker.observations = measurement.SeqIdx, 0
	tracker.entry, tracker.cost = tracker.ask, cost
	tracker.high, tracker.low = price, price
	tracker.highTick, tracker.lowTick = measurement.SeqIdx, measurement.SeqIdx
	return record, nil
}
