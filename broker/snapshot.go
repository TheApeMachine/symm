package broker

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/data"
)

/*
Level is one price level of a stamped book side.
*/
type Level struct {
	Price    *decimal.Decimal
	Quantity *decimal.Decimal
}

/*
Snapshot is a symbol's resident book as ingress stamped it onto an event: every
level of both sides inside the book's depth window, best first, and the window
size itself. It is taken under the symbol's read lock at the event's sequence
number, so it is exactly the book a SeqIdx-ordered replay of the stored level3
tape rebuilds at that event. A snapshot always holds an executable touch: an
unseeded, diverged, missing, one-sided, or crossed book is never stamped.

Every consumer of book state reads the snapshot its event carries, never the
live book, so a consumer that runs later than ingress still sees the book the
event happened against.
*/
type Snapshot struct {
	Depth int
	Bids  []Level
	Asks  []Level
}

const (
	snapshotSource = "spot:trade"
	snapshotDepth  = "book_depth"
)

/*
snapshotSides names each side's metric prefix with the order its prices must
keep from best to worst.
*/
var snapshotSides = [2]string{"bid", "ask"}

/*
Touch returns the snapshot's best bid and best ask.
*/
func (snapshot *Snapshot) Touch() (bid Level, ask Level) {
	return snapshot.Bids[0], snapshot.Asks[0]
}

/*
Full reports whether a side fills the depth window, so prices beyond its worst
level may rest outside the window rather than be absent from the book.
*/
func (snapshot *Snapshot) Full(levels []Level) bool {
	return len(levels) == snapshot.Depth
}

/*
Equal reports whether two snapshots hold the same window and the same levels to
the venue's exact digits.
*/
func (snapshot *Snapshot) Equal(other *Snapshot) bool {
	if snapshot == nil || other == nil {
		return snapshot == other
	}

	if snapshot.Depth != other.Depth {
		return false
	}

	return sameLevels(snapshot.Bids, other.Bids) && sameLevels(snapshot.Asks, other.Asks)
}

func sameLevels(left, right []Level) bool {
	if len(left) != len(right) {
		return false
	}

	for index := range left {
		if left[index].Price.Cmp(right[index].Price) != 0 {
			return false
		}

		if left[index].Quantity.Cmp(right[index].Quantity) != 0 {
			return false
		}
	}

	return true
}

/*
Metrics encodes the snapshot as trade metrics: book_depth, and for each side
and level index i the exact <side>_price_i and <side>_qty_i.
*/
func (snapshot *Snapshot) Metrics() []*data.Metric {
	unit, timescale := data.GetDimension(snapshotSource, snapshotDepth)
	metrics := make([]*data.Metric, 0, 1+2*(len(snapshot.Bids)+len(snapshot.Asks)))
	metrics = append(metrics, data.NewMetric(snapshotDepth, float64(snapshot.Depth), unit, timescale))

	for sideIndex, levels := range [2][]Level{snapshot.Bids, snapshot.Asks} {
		side := snapshotSides[sideIndex]

		for index, quote := range levels {
			priceLabel := fmt.Sprintf("%s_price_%d", side, index)
			qtyLabel := fmt.Sprintf("%s_qty_%d", side, index)
			priceUnit, priceTime := data.GetDimension(snapshotSource, priceLabel)
			qtyUnit, qtyTime := data.GetDimension(snapshotSource, qtyLabel)

			metrics = append(metrics,
				data.NewExactMetric(priceLabel, quote.Price, priceUnit, priceTime),
				data.NewExactMetric(qtyLabel, quote.Quantity, qtyUnit, qtyTime),
			)
		}
	}

	return metrics
}

/*
ReadSnapshot decodes the snapshot ingress stamped onto measurement. A
measurement stamped without a book returns nil and no error: the book was not
readable at that event. A stamp that is incomplete, out of order, crossed,
non-positive, or wider than its window is an error.
*/
func ReadSnapshot(measurement *data.Measurement) (*Snapshot, error) {
	var depth *data.Metric
	levels := map[string]map[int]*decimal.Decimal{}

	for entry := range measurement.Read() {
		if entry.Err != nil {
			return nil, errnie.Error(entry.Err)
		}

		if entry.Key == snapshotDepth {
			depth = entry.Metric
			continue
		}

		separator := strings.LastIndexByte(entry.Key, '_')

		if separator < 0 {
			continue
		}

		prefix := entry.Key[:separator]

		if !snapshotPrefix(prefix) {
			continue
		}

		index, err := strconv.Atoi(entry.Key[separator+1:])

		if err != nil {
			return nil, errnie.Error(errnie.Err(
				errnie.Validation, "[snapshot] level index unreadable: "+entry.Key, err,
			))
		}

		if entry.Metric.Exact() == nil {
			return nil, errnie.Error(errnie.Err(
				errnie.Validation, "[snapshot] level has no exact value: "+entry.Key, nil,
			))
		}

		if levels[prefix] == nil {
			levels[prefix] = map[int]*decimal.Decimal{}
		}

		levels[prefix][index] = entry.Metric.Exact()
	}

	if depth == nil {
		if len(levels) > 0 {
			return nil, errnie.Error(errnie.Err(
				errnie.Validation, "[snapshot] levels stamped without a depth window", nil,
			))
		}

		return nil, nil
	}

	snapshot := &Snapshot{Depth: int(depth.Raw())}

	if float64(snapshot.Depth) != depth.Raw() || snapshot.Depth <= 0 {
		return nil, errnie.Error(errnie.Err(
			errnie.Validation, fmt.Sprintf("[snapshot] depth window %v is not a positive count", depth.Raw()), nil,
		))
	}

	bids, err := snapshot.side(levels, "bid", 1)

	if err != nil {
		return nil, err
	}

	asks, err := snapshot.side(levels, "ask", -1)

	if err != nil {
		return nil, err
	}

	snapshot.Bids, snapshot.Asks = bids, asks

	if bids[0].Price.Cmp(asks[0].Price) >= 0 {
		return nil, errnie.Error(errnie.Err(
			errnie.Validation, "[snapshot] stamped touch is crossed", nil,
		))
	}

	return snapshot, nil
}

func snapshotPrefix(prefix string) bool {
	for _, side := range snapshotSides {
		if prefix == side+"_price" || prefix == side+"_qty" {
			return true
		}
	}

	return false
}

/*
side assembles one side's levels from contiguous level indices. order is the
sign each price must compare against the next worse one: +1 for bids, which
fall, and -1 for asks, which rise.
*/
func (snapshot *Snapshot) side(
	levels map[string]map[int]*decimal.Decimal, side string, order int,
) ([]Level, error) {
	prices, quantities := levels[side+"_price"], levels[side+"_qty"]

	if len(prices) == 0 || len(prices) != len(quantities) || len(prices) > snapshot.Depth {
		return nil, errnie.Error(errnie.Err(
			errnie.Validation,
			fmt.Sprintf("[snapshot] %s side holds %d prices and %d quantities in a window of %d",
				side, len(prices), len(quantities), snapshot.Depth),
			nil,
		))
	}

	ladder := make([]Level, len(prices))

	for index := range ladder {
		price, quantity := prices[index], quantities[index]

		if price == nil || quantity == nil {
			return nil, errnie.Error(errnie.Err(
				errnie.Validation, fmt.Sprintf("[snapshot] %s level %d is missing", side, index), nil,
			))
		}

		if price.Sign() <= 0 || quantity.Sign() <= 0 {
			return nil, errnie.Error(errnie.Err(
				errnie.Validation, fmt.Sprintf("[snapshot] %s level %d is not positive", side, index), nil,
			))
		}

		if index > 0 && ladder[index-1].Price.Cmp(price) != order {
			return nil, errnie.Error(errnie.Err(
				errnie.Validation, fmt.Sprintf("[snapshot] %s level %d is out of price order", side, index), nil,
			))
		}

		ladder[index] = Level{Price: price, Quantity: quantity}
	}

	return ladder, nil
}
