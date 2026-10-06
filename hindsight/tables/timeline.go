package tables

import (
	"cmp"
	"context"
	"iter"
	"slices"

	"github.com/apache/iceberg-go"
	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/data"
)

/*
Trades efficiently retrieves measurements from the measurements table where
source = "spot:trade", grouped by label, and sorted within each label by epoch
ascending and tick ascending (with seqIdx as tie-breaker).

A read failure (including a failed validation of the arguments) is yielded as
a non-nil error and ends the sequence; it is never reported as end-of-stream.
*/
func (catalog *Catalog) Trades(
	ctx context.Context,
	epoch ...int64,
) iter.Seq2[*data.Measurement, error] {
	return func(yield func(*data.Measurement, error) bool) {
		if catalog == nil {
			yield(nil, errnie.Error(errnie.Err(
				errnie.Validation,
				"[catalog] catalog is required",
				nil,
			)))
			return
		}

		targetEpoch := int64(0)

		if len(epoch) > 0 && epoch[0] > 0 {
			targetEpoch = epoch[0]
		}

		filter := iceberg.BooleanExpression(
			iceberg.EqualTo(iceberg.Reference("source"), "spot:trade"),
		)

		grouped := make(map[string][]*data.Measurement)

		for measurement, err := range catalog.scan(ctx, Measurements, targetEpoch, filter, 0) {
			if err != nil {
				yield(nil, err)
				return
			}

			if measurement == nil {
				continue
			}

			grouped[measurement.Label] = append(grouped[measurement.Label], measurement)
		}

		labels := make([]string, 0, len(grouped))

		for label := range grouped {
			labels = append(labels, label)
		}

		slices.Sort(labels)

		for _, label := range labels {
			items := grouped[label]

			slices.SortFunc(items, func(left, right *data.Measurement) int {
				if cmpResult := cmp.Compare(left.Epoch, right.Epoch); cmpResult != 0 {
					return cmpResult
				}

				if cmpResult := cmp.Compare(left.Tick, right.Tick); cmpResult != 0 {
					return cmpResult
				}

				return cmp.Compare(left.SeqIdx, right.SeqIdx)
			})

			for _, measurement := range items {
				if !yield(measurement, nil) {
					return
				}
			}
		}
	}
}

/*
Detections efficiently retrieves measurements from the measurements table where
source = "detector", optionally filtered by epoch, grouped by label, and sorted
within each label by epoch ascending and tick ascending (with seqIdx as tie-breaker).

A read failure (including a failed validation of the arguments) is yielded as
a non-nil error and ends the sequence; it is never reported as end-of-stream.
*/
func (catalog *Catalog) Detections(
	ctx context.Context,
	epoch ...int64,
) iter.Seq2[*data.Measurement, error] {
	return func(yield func(*data.Measurement, error) bool) {
		if catalog == nil {
			yield(nil, errnie.Error(errnie.Err(
				errnie.Validation,
				"[catalog] catalog is required",
				nil,
			)))
			return
		}

		targetEpoch := int64(0)

		if len(epoch) > 0 && epoch[0] > 0 {
			targetEpoch = epoch[0]
		}

		filter := iceberg.BooleanExpression(
			iceberg.EqualTo(iceberg.Reference("source"), "detector"),
		)

		grouped := make(map[string][]*data.Measurement)

		for measurement, err := range catalog.scan(ctx, Measurements, targetEpoch, filter, 0) {
			if err != nil {
				yield(nil, err)
				return
			}

			if measurement == nil {
				continue
			}

			if measurement.Source != "detector" {
				continue
			}

			grouped[measurement.Label] = append(grouped[measurement.Label], measurement)
		}

		labels := make([]string, 0, len(grouped))

		for label := range grouped {
			labels = append(labels, label)
		}

		slices.Sort(labels)

		for _, label := range labels {
			items := grouped[label]

			slices.SortFunc(items, func(left, right *data.Measurement) int {
				if cmpResult := cmp.Compare(left.Epoch, right.Epoch); cmpResult != 0 {
					return cmpResult
				}

				if cmpResult := cmp.Compare(left.Tick, right.Tick); cmpResult != 0 {
					return cmpResult
				}

				return cmp.Compare(left.SeqIdx, right.SeqIdx)
			})

			for _, measurement := range items {
				if !yield(measurement, nil) {
					return
				}
			}
		}
	}
}

/*
Excursions is an alias for Detections, retrieving measurements where source = "detector".
*/
func (catalog *Catalog) Excursions(
	ctx context.Context,
	epoch ...int64,
) iter.Seq2[*data.Measurement, error] {
	return catalog.Detections(ctx, epoch...)
}

/*
SensorySources enumerates the canonical Stage 0 signal producer sources.
*/
var SensorySources = []string{
	"correlation",
	"cvd",
	"depthflow",
	"hawkes",
	"leadlag",
	"liquidity",
	"morphology",
	"pumpdump",
	"sentiment",
	"toxicity",
}

/*
LogicSources enumerates the canonical Stage 1 cognitive and physical solver sources.
*/
var LogicSources = []string{
	"resonance",
	"manifold",
}

/*
SignalLogicSources enumerates the canonical signal and logic producer sources.
*/
var SignalLogicSources = append(slices.Clone(SensorySources), LogicSources...)

/*
DetectionTicks extracts the start, B, and C ticks recorded in a detector
measurement. B is ignition (or the start of a chop/flat stretch) and C is
exhaustion (or its end); start precedes or equals B, and B precedes C.
*/
func DetectionTicks(measurement *data.Measurement) (int64, int64, int64, error) {
	if measurement == nil {
		return 0, 0, 0, errnie.Error(errnie.Err(
			errnie.Validation,
			"[catalog] measurement is required",
			nil,
		))
	}

	ticks := make([]int64, 0, 3)

	for _, key := range []string{"start_tick", "b_tick", "c_tick"} {
		entry := data.Pull(measurement.Read(key))

		if entry == nil || entry.Err != nil || entry.Metric == nil || entry.Metric.Label != key {
			return 0, 0, 0, errnie.Error(errnie.Err(
				errnie.Validation,
				"[catalog] detection measurement missing "+key,
				entryErr(entry),
			))
		}

		ticks = append(ticks, int64(entry.Metric.Raw))
	}

	if ticks[0] < 0 || ticks[0] > ticks[1] || ticks[1] >= ticks[2] {
		return 0, 0, 0, errnie.Error(errnie.Err(
			errnie.Validation,
			"[catalog] invalid tick range in detection measurement",
			nil,
		))
	}

	return ticks[0], ticks[1], ticks[2], nil
}

/*
entryErr returns the read error carried by entry, if any. A key the
measurement never wrote yields a nil entry and so a nil error; the caller's
"missing" message carries that case.
*/
func entryErr(entry *data.MetricEntry) error {
	if entry == nil {
		return nil
	}

	return entry.Err
}

/*
DetectionPrices extracts the B and C prices recorded in a detector
measurement. C may be above B (up classes), below it (down), or near it
(chop/flat).
*/
func DetectionPrices(measurement *data.Measurement) (*decimal.Decimal, *decimal.Decimal, error) {
	if measurement == nil {
		return nil, nil, errnie.Error(errnie.Err(
			errnie.Validation,
			"[catalog] measurement is required",
			nil,
		))
	}

	prices := make([]*decimal.Decimal, 0, 2)

	for _, key := range []string{"b_price", "c_price"} {
		entry := data.Pull(measurement.Read(key))

		if entry == nil || entry.Err != nil || entry.Metric == nil || entry.Metric.Label != key {
			return nil, nil, errnie.Error(errnie.Err(
				errnie.Validation,
				"[catalog] detection measurement missing "+key,
				entryErr(entry),
			))
		}

		price := entry.Metric.Exact

		if price == nil || price.Sign() <= 0 {
			return nil, nil, errnie.Error(errnie.Err(
				errnie.Validation,
				"[catalog] detection measurement has no positive exact "+key,
				nil,
			))
		}

		prices = append(prices, price)
	}

	return prices[0], prices[1], nil
}

/*
SignalLogic retrieves signal and logic measurements for a specific epoch, label,
and tick range [lowTick, highTick], sorted chronologically by tick ascending,
with seqIdx and source as tie-breakers.

A read failure (including a failed validation of the arguments) is yielded as
a non-nil error and ends the sequence; it is never reported as end-of-stream.
*/
func (catalog *Catalog) SignalLogic(
	ctx context.Context,
	epoch int64,
	label string,
	lowTick int64,
	highTick int64,
	sources ...string,
) iter.Seq2[*data.Measurement, error] {
	return func(yield func(*data.Measurement, error) bool) {
		if catalog == nil {
			yield(nil, errnie.Error(errnie.Err(
				errnie.Validation,
				"[catalog] catalog is required",
				nil,
			)))
			return
		}

		if epoch <= 0 {
			yield(nil, errnie.Error(errnie.Err(
				errnie.Validation,
				"[catalog] valid positive epoch is required",
				nil,
			)))
			return
		}

		if label == "" {
			yield(nil, errnie.Error(errnie.Err(
				errnie.Validation,
				"[catalog] label is required",
				nil,
			)))
			return
		}

		if lowTick < 0 {
			yield(nil, errnie.Error(errnie.Err(
				errnie.Validation,
				"[catalog] lowTick must be non-negative",
				nil,
			)))
			return
		}

		if highTick > 0 && lowTick > highTick {
			yield(nil, errnie.Error(errnie.Err(
				errnie.Validation,
				"[catalog] lowTick cannot exceed highTick",
				nil,
			)))
			return
		}

		targetSources := SignalLogicSources

		if len(sources) > 0 {
			targetSources = sources
		}

		var sourceFilter iceberg.BooleanExpression

		if len(targetSources) == 1 {
			sourceFilter = iceberg.EqualTo(iceberg.Reference("source"), targetSources[0])
		}

		if len(targetSources) > 1 {
			sourceFilter = iceberg.IsIn(iceberg.Reference("source"), targetSources...)
		}

		filter := iceberg.BooleanExpression(iceberg.EqualTo(iceberg.Reference("label"), label))

		if sourceFilter != nil {
			filter = iceberg.NewAnd(filter, sourceFilter)
		}

		if lowTick > 0 {
			filter = iceberg.NewAnd(
				filter,
				iceberg.GreaterThanEqual(iceberg.Reference("tick"), lowTick),
			)
		}

		if highTick > 0 && highTick >= lowTick {
			filter = iceberg.NewAnd(
				filter,
				iceberg.LessThanEqual(iceberg.Reference("tick"), highTick),
			)
		}

		sourceLookup := make(map[string]struct{}, len(targetSources))

		for _, src := range targetSources {
			sourceLookup[src] = struct{}{}
		}

		var results []*data.Measurement

		for measurement, err := range catalog.scan(ctx, Measurements, epoch, filter, 0) {
			if err != nil {
				yield(nil, err)
				return
			}

			if measurement == nil {
				continue
			}

			if measurement.Label != label {
				continue
			}

			if _, exists := sourceLookup[measurement.Source]; !exists {
				continue
			}

			if lowTick > 0 && measurement.Tick < lowTick {
				continue
			}

			if highTick > 0 && measurement.Tick > highTick {
				continue
			}

			results = append(results, measurement)
		}

		slices.SortFunc(results, func(left, right *data.Measurement) int {
			if cmpResult := cmp.Compare(left.Tick, right.Tick); cmpResult != 0 {
				return cmpResult
			}

			if cmpResult := cmp.Compare(left.SeqIdx, right.SeqIdx); cmpResult != 0 {
				return cmpResult
			}

			return cmp.Compare(left.Source, right.Source)
		})

		for _, measurement := range results {
			if !yield(measurement, nil) {
				return
			}
		}
	}
}

/*
ExcursionTape retrieves the signal and logic tape for an excursion window.
*/
func (catalog *Catalog) ExcursionTape(
	ctx context.Context,
	epoch int64,
	label string,
	lowTick int64,
	highTick int64,
	sources ...string,
) iter.Seq2[*data.Measurement, error] {
	return catalog.SignalLogic(ctx, epoch, label, lowTick, highTick, sources...)
}

/*
Timeline reconstructs the sequential market tape from the unified Measurements table.

The window [fromTick, toTick] is in market ticks only (a bound <= 0 is open).
Rows are yielded in sequence-index order. To address a single frame by its
sequence index, use Scan with a seqIdx predicate instead.

A read failure is yielded as a non-nil error and ends the sequence; it is never
reported as end-of-stream.
*/
func (catalog *Catalog) Timeline(
	ctx context.Context,
	epoch int64,
	label string,
	fromTick int64,
	toTick int64,
) iter.Seq2[*data.Measurement, error] {
	var filter iceberg.BooleanExpression

	and := func(expression iceberg.BooleanExpression) {
		if filter == nil {
			filter = expression
			return
		}

		filter = iceberg.NewAnd(filter, expression)
	}

	if label != "" {
		and(iceberg.EqualTo(iceberg.Reference("label"), label))
	}

	if fromTick > 0 {
		and(iceberg.GreaterThanEqual(iceberg.Reference("tick"), fromTick))
	}

	if toTick > 0 && toTick >= fromTick {
		and(iceberg.LessThanEqual(iceberg.Reference("tick"), toTick))
	}

	return func(yield func(*data.Measurement, error) bool) {
		for measurement, err := range catalog.Scan(ctx, Measurements, epoch, filter, 0) {
			if err != nil {
				yield(nil, err)
				return
			}

			// The pushed-down predicate may only prune files; enforce the
			// window on every row so no out-of-window frame leaks through.
			if label != "" && measurement.Label != label {
				continue
			}

			if fromTick > 0 && measurement.Tick < fromTick {
				continue
			}

			if toTick > 0 && toTick >= fromTick && measurement.Tick > toTick {
				continue
			}

			if !yield(measurement, nil) {
				return
			}
		}
	}
}

/*
Labels discovers distinct labels present in an epoch by reading only the label column.
*/
func (catalog *Catalog) Labels(ctx context.Context, epoch int64) ([]string, error) {
	labelSet := make(map[string]struct{})

	for measurement, err := range catalog.scan(ctx, Measurements, epoch, nil, 0, "label") {
		if err != nil {
			return nil, err
		}

		if measurement != nil && measurement.Label != "" {
			labelSet[measurement.Label] = struct{}{}
		}
	}

	labels := make([]string, 0, len(labelSet))

	for label := range labelSet {
		labels = append(labels, label)
	}

	slices.Sort(labels)

	return labels, nil
}
