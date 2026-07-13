package manifold

import (
	"strings"
	"time"

	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/types"
)

/*
ObservationMetadata identifies the authoritative L3 rows accumulated into one
field advance without retaining the row's order slices.
*/
type ObservationMetadata struct {
	At        time.Time
	FrameType string
	Checksum  uint32
	Count     int
}

/*
pendingObservation is the latest valid market boundary for the population that
has not yet advanced the field solver.
*/
type pendingObservation struct {
	metadata ObservationMetadata
	bestBid  float64
	bestAsk  float64
	midPrice float64
}

/*
Observe validates and applies one authoritative L3 row without advancing the
GPU field. Every accepted row updates the exact population ledger.
*/
func (slot *Slot) Observe(
	thesis *types.Thesis,
	row kraken.Level3Data,
	pricePrecision int,
	qtyPrecision int,
	book Level3Book,
) ProcessResult {
	observation := ObservationMetadata{
		At:        row.Timestamp,
		FrameType: row.Type,
		Checksum:  row.Checksum,
		Count:     1,
	}

	if slot.timestampRegressed(row.Timestamp) {
		return slot.invalidate(thesis, observation, TimestampRegress)
	}

	if book == nil {
		return slot.invalidate(thesis, observation, BookInvalid)
	}

	if !book.Apply(row, pricePrecision, qtyPrecision) {
		reason := book.InvalidReason(slot.symbol)

		if reason == Valid {
			reason = BookInvalid
		}

		return slot.invalidate(thesis, observation, reason)
	}

	bestBid, bestAsk, ok := book.TopOfBook(slot.symbol)

	if !ok {
		return slot.invalidate(thesis, observation, NonPositiveMid)
	}

	midPrice := (bestBid + bestAsk) / 2
	slot.population.Apply(row, midPrice)

	if !slot.population.Ready() {
		return slot.invalidate(thesis, observation, slot.population.InvalidReason())
	}

	if observation.At.IsZero() {
		observation.At = slot.population.LastAt()
	}

	if slot.timestampRegressed(observation.At) {
		return slot.invalidate(thesis, observation, TimestampRegress)
	}

	if slot.advanceReady && !strings.EqualFold(row.Type, "snapshot") {
		observation.Count = slot.pending.metadata.Count + 1
	}

	slot.pending = pendingObservation{
		metadata: observation,
		bestBid:  bestBid,
		bestAsk:  bestAsk,
		midPrice: midPrice,
	}
	slot.advanceReady = true

	if !observation.At.IsZero() {
		slot.lastObservedAt = observation.At
	}

	return ProcessResult{
		Observation:  observation,
		Accounting:   slot.population.Accounting(),
		AdvanceReady: true,
	}
}

func (slot *Slot) timestampRegressed(at time.Time) bool {
	return !at.IsZero() && !slot.lastObservedAt.IsZero() && at.Before(slot.lastObservedAt)
}

func (slot *Slot) invalidate(
	thesis *types.Thesis,
	observation ObservationMetadata,
	reason InvalidReason,
) ProcessResult {
	slot.pending = pendingObservation{}
	slot.advanceReady = false
	slot.population.invalidate(reason)

	return slot.failedResult(thesis, observation, reason)
}
