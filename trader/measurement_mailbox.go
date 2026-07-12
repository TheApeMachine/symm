package trader

import (
	"fmt"
	"sync/atomic"

	"github.com/theapemachine/symm/types"
)

type measurementKey struct {
	symbol string
	source types.SourceType
	stream string
}

/*
MeasurementMailbox publishes the newest immutable measurement for each
symbol, source, and stream from one producer to one scanning consumer.
*/
type MeasurementMailbox struct {
	keys       map[measurementKey]int
	slots      []atomic.Pointer[types.Measurement]
	next       int
	superseded atomic.Uint64
}

/*
NewMeasurementMailbox allocates the fixed number of measurement identities
the admitted trading tier can produce.
*/
func NewMeasurementMailbox(capacity int) (*MeasurementMailbox, error) {
	if capacity <= 0 {
		return nil, fmt.Errorf("trader: measurement mailbox capacity must be positive")
	}

	return &MeasurementMailbox{
		keys:  make(map[measurementKey]int, capacity),
		slots: make([]atomic.Pointer[types.Measurement], capacity),
	}, nil
}

/*
Store publishes measurement after assigning its producer-owned identity slot.
Replacing an unread value is explicit because active Thesis evidence retains
one current value for the same source.
*/
func (mailbox *MeasurementMailbox) Store(measurement *types.Measurement) error {
	key, err := mailbox.key(measurement)

	if err != nil {
		return err
	}

	index, ok := mailbox.keys[key]

	if !ok {
		if mailbox.next >= len(mailbox.slots) {
			return fmt.Errorf("trader: measurement mailbox capacity exhausted")
		}

		index = mailbox.next
		mailbox.next++
		mailbox.keys[key] = index
	}

	if mailbox.slots[index].Swap(measurement) != nil {
		mailbox.superseded.Add(1)
	}

	return nil
}

/*
Drain atomically takes every currently published measurement.
*/
func (mailbox *MeasurementMailbox) Drain() []*types.Measurement {
	count := 0

	for index := range mailbox.slots {
		if mailbox.slots[index].Load() != nil {
			count++
		}
	}

	if count == 0 {
		return nil
	}

	measurements := make([]*types.Measurement, 0, count)

	for index := range mailbox.slots {
		measurement := mailbox.slots[index].Swap(nil)

		if measurement != nil {
			measurements = append(measurements, measurement)
		}
	}

	return measurements
}

/*
Superseded reports current measurements replaced before the consumer read them.
*/
func (mailbox *MeasurementMailbox) Superseded() uint64 {
	return mailbox.superseded.Load()
}

func (mailbox *MeasurementMailbox) key(
	measurement *types.Measurement,
) (measurementKey, error) {
	if measurement == nil {
		return measurementKey{}, fmt.Errorf("trader: measurement mailbox value required")
	}

	if measurement.Symbol == "" || measurement.Source == "" || measurement.Stream == "" {
		return measurementKey{}, fmt.Errorf(
			"trader: measurement mailbox symbol, source, and stream required",
		)
	}

	return measurementKey{
		symbol: measurement.Symbol,
		source: measurement.Source,
		stream: measurement.Stream,
	}, nil
}
