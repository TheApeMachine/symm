package strategy

import (
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"time"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/data"
)

/*
Position frames are paper trading's live round trips on the hub (data.
PositionOrigin): one frame per thing that happened to a trip (an Event), and
a mark of every open position against the live bid on each tick (a Mark).
Only what the paper desk actually did is published: the policy match that
entered, the sizing, each order submitted, each fill, the policy match that
exited (the trigger), the close and its outcome, and orders that failed or
were canceled. Paper trading has no real-exchange ledger, so no frame
carries one.
*/

/*
Trip statuses.
*/
const (
	tripEntering  = "entering"
	tripHolding   = "holding"
	tripExiting   = "exiting"
	tripClosed    = "closed"
	tripAbandoned = "abandoned"
)

/*
PositionEvent is one thing that happened to a round trip: its kind, when,
what it was, and the values it carried.
*/
type PositionEvent struct {
	At     time.Time      `json:"at"`
	Kind   string         `json:"kind"`
	Detail string         `json:"detail"`
	Fields map[string]any `json:"fields,omitempty"`
}

/*
PositionMark is an open position valued at the live bid: what selling the
filled quantity at the bid would realize, after the fees paid so far.
*/
type PositionMark struct {
	At               time.Time `json:"at"`
	Bid              float64   `json:"bid"`
	Quantity         float64   `json:"quantity"`
	EntryPrice       float64   `json:"entry_price"`
	Unrealized       float64   `json:"unrealized"`
	UnrealizedReturn float64   `json:"unrealized_return"`
}

/*
PositionOutcome is a closed round trip as the paper desk filled it.
ExpectedHoldNs is the entry policy's stored mean hold (null when the policy
stores none). Triggers name what closed it.
*/
type PositionOutcome struct {
	Cost           float64  `json:"cost"`
	Proceeds       float64  `json:"proceeds"`
	Fees           float64  `json:"fees"`
	Realized       float64  `json:"realized"`
	NetReturn      float64  `json:"net_return"`
	HoldNs         int64    `json:"hold_ns"`
	ExpectedHoldNs *int64   `json:"expected_hold_ns"`
	Triggers       []string `json:"triggers"`
}

/*
PositionFrame is one round trip's state after an Event or a Mark (exactly
one is set). ID names the trip: its symbol and the entry decision's time.
*/
type PositionFrame struct {
	ID          string           `json:"id"`
	Symbol      string           `json:"symbol"`
	Status      string           `json:"status"`
	PolicyEpoch uint64           `json:"policy_epoch,string"`
	OpenedAt    time.Time        `json:"opened_at"`
	ClosedAt    *time.Time       `json:"closed_at"`
	Event       *PositionEvent   `json:"event"`
	Mark        *PositionMark    `json:"mark"`
	Outcome     *PositionOutcome `json:"outcome"`
}

/*
tripID names a round trip by its symbol and its entry decision's time.
*/
func tripID(symbol string, trip *paperTrip) string {
	return symbol + "@" + strconv.FormatInt(trip.entry.decidedAt.UnixNano(), 10)
}

/*
publish writes frame to the hub as a data.PositionOrigin Value.
*/
func (training *Training) publish(frame PositionFrame) error {
	role := "mark"

	if frame.Event != nil {
		role = frame.Event.Kind
	}

	payload, err := json.Marshal(frame)

	if err != nil {
		return errnie.Err(errnie.Internal, "[training] position frame of "+frame.ID, err)
	}

	value := data.NewValue(data.PositionOrigin, role, frame.Symbol)
	value.Epoch, value.Tick = training.tree.Epoch(), training.seq.Add(1)
	value.FormatPrefix()
	value.AppendJSON(payload, value.Prefix+"/"+data.LayerPosition)

	if _, err := io.Copy(training.hub, value); err != nil {
		return errnie.Err(errnie.IO, "[training] position frame "+value.Prefix+" to the hub", err)
	}

	return nil
}

/*
event moves trip to status and publishes what happened.
*/
func (training *Training) event(symbol string, trip *paperTrip, status, kind, detail string, fields map[string]any) error {
	trip.status = status

	return training.publish(PositionFrame{
		ID: tripID(symbol, trip), Symbol: symbol, Status: status, PolicyEpoch: training.tree.Epoch(),
		OpenedAt: trip.entry.decidedAt, ClosedAt: trip.closedAt,
		Event: &PositionEvent{At: time.Now(), Kind: kind, Detail: detail, Fields: fields},
	})
}

/*
matchFields are a policy match's values: the path, its depth, and what the
stored policy for action holds.
*/
func (training *Training) matchFields(match CognitionMatch, action string) map[string]any {
	fields := map[string]any{"path": match.Path, "depth": match.Depth}

	if metrics, ok := training.tree.Metrics(match.Path, action); ok {
		for key, value := range metrics {
			fields[key] = value
		}
	}

	return fields
}

/*
Mark values symbol's open position at the live bid and publishes it; a
symbol with no filled entry this run is not marked. A held position with
no bid to value it at is an error: its value is unknown, not zero.
*/
func (training *Training) Mark(symbol string) error {
	training.mu.Lock()
	defer training.mu.Unlock()

	trip := training.trips[symbol]

	if trip == nil || trip.entry == nil || !trip.entry.filled {
		return nil
	}

	bid, _ := training.price.Touch(symbol)

	if bid == nil || bid.Sign() <= 0 {
		return errnie.Err(errnie.NotFound, "[training] no bid to mark the open "+symbol+" position at", nil)
	}

	entry := trip.entry
	value := entry.qty * bid.Float64()
	unrealized := value - entry.cost - entry.fee

	return training.publish(PositionFrame{
		ID: tripID(symbol, trip), Symbol: symbol, Status: trip.status, PolicyEpoch: training.tree.Epoch(),
		OpenedAt: entry.decidedAt,
		Mark: &PositionMark{
			At: time.Now(), Bid: bid.Float64(), Quantity: entry.qty, EntryPrice: entry.cost / entry.qty,
			Unrealized: unrealized, UnrealizedReturn: unrealized / (entry.cost + entry.fee),
		},
	})
}

/*
closeTrip publishes the closed round trip's outcome.
*/
func (training *Training) closeTrip(symbol string, trip *paperTrip, pnl, netReturn float64) error {
	entry, exit := trip.entry, trip.exit
	closed := exit.filledAt
	trip.closedAt = &closed
	trip.status = tripClosed

	outcome := &PositionOutcome{
		Cost: entry.cost + entry.fee, Proceeds: exit.cost - exit.fee, Fees: entry.fee + exit.fee,
		Realized: pnl, NetReturn: netReturn, HoldNs: exit.filledAt.Sub(entry.filledAt).Nanoseconds(),
		Triggers: []string{exitTrigger(exit.match)},
	}

	if metrics, ok := training.tree.Metrics(entry.match.Path, "enter"); ok {
		if hold, stored := metrics["hold_seconds"]; stored {
			expected := int64(hold * float64(time.Second))
			outcome.ExpectedHoldNs = &expected
		}
	}

	return training.publish(PositionFrame{
		ID: tripID(symbol, trip), Symbol: symbol, Status: tripClosed, PolicyEpoch: training.tree.Epoch(),
		OpenedAt: entry.decidedAt, ClosedAt: trip.closedAt,
		Event: &PositionEvent{
			At: time.Now(), Kind: "closed",
			Detail: fmt.Sprintf("closed: pnl %.6f %s (net return %.5f)", pnl, training.quote, netReturn),
		},
		Outcome: outcome,
	})
}

/*
exitTrigger names what closed a round trip: paper trading exits only on a
stored policy's exit match.
*/
func exitTrigger(match CognitionMatch) string {
	return fmt.Sprintf("policy exit match %s (depth %d)", match.Path, match.Depth)
}
