package toxicity

import (
	"time"

	"github.com/theapemachine/symm/kraken/market"
)

func (tox *Toxicity) observeLevel3(update *market.Level3Update) {
	if update == nil {
		return
	}

	now := time.Now()
	frameTime := market.Level3EventTime(update.Timestamp, now)

	for _, event := range update.Bids {
		tox.applyLevel3Event(update.Symbol, SideBid, event, frameTime, now)
	}

	for _, event := range update.Asks {
		tox.applyLevel3Event(update.Symbol, SideAsk, event, frameTime, now)
	}

	tox.publishMeasurement(update.Symbol, 0)
}

func (tox *Toxicity) applyLevel3Event(
	symbol string,
	side byte,
	event market.Level3OrderEvent,
	eventTime, now time.Time,
) {
	if event.OrderID == "" {
		return
	}

	ts := market.Level3EventTime(event.Timestamp, eventTime)

	tox.tracker.ApplyOrder(
		symbol,
		market.Pair{},
		event.Event,
		event.OrderID,
		side,
		event.LimitPrice,
		event.OrderQty,
		ts,
		now,
	)
}
