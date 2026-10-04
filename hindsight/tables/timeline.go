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
*/
func (catalog *Catalog) Trades(
	ctx context.Context,
	epoch ...int64,
) iter.Seq[*data.Measurement[float64]] {
	return func(yield func(*data.Measurement[float64]) bool) {
		if catalog == nil {
			errnie.Error(errnie.Err(
				errnie.Validation,
				"[catalog] catalog is required",
				nil,
			))
			return
		}

		targetEpoch := int64(0)

		if len(epoch) > 0 && epoch[0] > 0 {
			targetEpoch = epoch[0]
		}

		filter := iceberg.BooleanExpression(
			iceberg.EqualTo(iceberg.Reference("source"), "spot:trade"),
		)

		grouped := make(map[string][]*data.Measurement[float64])

		for measurement, err := range catalog.scan(ctx, Measurements, targetEpoch, filter, 0) {
			if err != nil {
				errnie.Error(err)
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

			slices.SortFunc(items, func(left, right *data.Measurement[float64]) int {
				if cmpResult := cmp.Compare(left.Epoch, right.Epoch); cmpResult != 0 {
					return cmpResult
				}

				if cmpResult := cmp.Compare(left.Tick, right.Tick); cmpResult != 0 {
					return cmpResult
				}

				return cmp.Compare(left.SeqIdx, right.SeqIdx)
			})

			for _, measurement := range items {
				if !yield(measurement) {
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
*/
func (catalog *Catalog) Detections(
	ctx context.Context,
	epoch ...int64,
) iter.Seq[*data.Measurement[float64]] {
	return func(yield func(*data.Measurement[float64]) bool) {
		if catalog == nil {
			errnie.Error(errnie.Err(
				errnie.Validation,
				"[catalog] catalog is required",
				nil,
			))
			return
		}

		targetEpoch := int64(0)

		if len(epoch) > 0 && epoch[0] > 0 {
			targetEpoch = epoch[0]
		}

		filter := iceberg.BooleanExpression(
			iceberg.EqualTo(iceberg.Reference("source"), "detector"),
		)

		grouped := make(map[string][]*data.Measurement[float64])

		for measurement, err := range catalog.scan(ctx, Measurements, targetEpoch, filter, 0) {
			if err != nil {
				errnie.Error(err)
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

			slices.SortFunc(items, func(left, right *data.Measurement[float64]) int {
				if cmpResult := cmp.Compare(left.Epoch, right.Epoch); cmpResult != 0 {
					return cmpResult
				}

				if cmpResult := cmp.Compare(left.Tick, right.Tick); cmpResult != 0 {
					return cmpResult
				}

				return cmp.Compare(left.SeqIdx, right.SeqIdx)
			})

			for _, measurement := range items {
				if !yield(measurement) {
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
) iter.Seq[*data.Measurement[float64]] {
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
DetectionTicks extracts the lowTick and highTick recorded in a detector measurement.
*/
func DetectionTicks(measurement *data.Measurement[float64]) (int64, int64, error) {
	if measurement == nil {
		return 0, 0, errnie.Error(errnie.Err(
			errnie.Validation,
			"[catalog] measurement is required",
			nil,
		))
	}

	lowMetric, hasLow := measurement.LookupMetric("LowTick")

	if !hasLow {
		lowMetric, hasLow = measurement.LookupMetric("low_tick")
	}

	highMetric, hasHigh := measurement.LookupMetric("HighTick")

	if !hasHigh {
		highMetric, hasHigh = measurement.LookupMetric("high_tick")
	}

	if !hasLow || !hasHigh {
		return 0, 0, errnie.Error(errnie.Err(
			errnie.Validation,
			"[catalog] detection measurement missing LowTick or HighTick metric",
			nil,
		))
	}

	lowTick := int64(lowMetric.Raw)
	highTick := int64(highMetric.Raw)

	if lowTick < 0 || (highTick > 0 && highTick < lowTick) {
		return 0, 0, errnie.Error(errnie.Err(
			errnie.Validation,
			"[catalog] invalid tick range in detection measurement",
			nil,
		))
	}

	return lowTick, highTick, nil
}

/*
DetectionPrices extracts the entry and exit prices recorded in a detector measurement.
*/
func DetectionPrices(measurement *data.Measurement[float64]) (*decimal.Decimal, *decimal.Decimal, error) {
	if measurement == nil {
		return nil, nil, errnie.Error(errnie.Err(
			errnie.Validation,
			"[catalog] measurement is required",
			nil,
		))
	}

	lowMetric, hasLow := measurement.LookupMetric("LowPrice")

	if !hasLow {
		lowMetric, hasLow = measurement.LookupMetric("low_price")
	}

	highMetric, hasHigh := measurement.LookupMetric("HighPrice")

	if !hasHigh {
		highMetric, hasHigh = measurement.LookupMetric("high_price")
	}

	if !hasLow || !hasHigh {
		return nil, nil, errnie.Error(errnie.Err(
			errnie.Validation,
			"[catalog] detection measurement missing LowPrice or HighPrice metric",
			nil,
		))
	}

	var entryAsk, exitBid *decimal.Decimal

	if lowMetric.Exact != nil {
		entryAsk = lowMetric.Exact
	}

	if entryAsk == nil && lowMetric.Raw > 0 {
		entryAsk = decimal.NewFromFloat64(lowMetric.Raw)
	}

	if highMetric.Exact != nil {
		exitBid = highMetric.Exact
	}

	if exitBid == nil && highMetric.Raw > 0 {
		exitBid = decimal.NewFromFloat64(highMetric.Raw)
	}

	if entryAsk == nil || exitBid == nil || entryAsk.Sign() <= 0 || exitBid.Sign() <= 0 {
		return nil, nil, errnie.Error(errnie.Err(
			errnie.Validation,
			"[catalog] invalid prices in detection measurement",
			nil,
		))
	}

	return entryAsk, exitBid, nil
}

/*
SignalLogic retrieves signal and logic measurements for a specific epoch, label,
and tick range [lowTick, highTick], sorted chronologically by tick ascending,
with seqIdx and source as tie-breakers.
*/
func (catalog *Catalog) SignalLogic(
	ctx context.Context,
	epoch int64,
	label string,
	lowTick int64,
	highTick int64,
	sources ...string,
) iter.Seq[*data.Measurement[float64]] {
	return func(yield func(*data.Measurement[float64]) bool) {
		if catalog == nil {
			errnie.Error(errnie.Err(
				errnie.Validation,
				"[catalog] catalog is required",
				nil,
			))
			return
		}

		if epoch <= 0 {
			errnie.Error(errnie.Err(
				errnie.Validation,
				"[catalog] valid positive epoch is required",
				nil,
			))
			return
		}

		if label == "" {
			errnie.Error(errnie.Err(
				errnie.Validation,
				"[catalog] label is required",
				nil,
			))
			return
		}

		if lowTick < 0 {
			errnie.Error(errnie.Err(
				errnie.Validation,
				"[catalog] lowTick must be non-negative",
				nil,
			))
			return
		}

		if highTick > 0 && lowTick > highTick {
			errnie.Error(errnie.Err(
				errnie.Validation,
				"[catalog] lowTick cannot exceed highTick",
				nil,
			))
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

		var results []*data.Measurement[float64]

		for measurement, err := range catalog.scan(ctx, Measurements, epoch, filter, 0) {
			if err != nil {
				errnie.Error(err)
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

		slices.SortFunc(results, func(left, right *data.Measurement[float64]) int {
			if cmpResult := cmp.Compare(left.Tick, right.Tick); cmpResult != 0 {
				return cmpResult
			}

			if cmpResult := cmp.Compare(left.SeqIdx, right.SeqIdx); cmpResult != 0 {
				return cmpResult
			}

			return cmp.Compare(left.Source, right.Source)
		})

		for _, measurement := range results {
			if !yield(measurement) {
				return
			}
		}
	}
}

/*
DetectionSignalLogic retrieves signal and logic measurements using the excursion parameters
recorded inside a detector measurement.
*/
func (catalog *Catalog) DetectionSignalLogic(
	ctx context.Context,
	detection *data.Measurement[float64],
	sources ...string,
) iter.Seq[*data.Measurement[float64]] {
	return func(yield func(*data.Measurement[float64]) bool) {
		if detection == nil {
			errnie.Error(errnie.Err(
				errnie.Validation,
				"[catalog] detection measurement is required",
				nil,
			))
			return
		}

		lowTick, highTick, err := DetectionTicks(detection)

		if err != nil {
			errnie.Error(err)
			return
		}

		for measurement := range catalog.SignalLogic(
			ctx,
			detection.Epoch,
			detection.Label,
			lowTick,
			highTick,
			sources...,
		) {
			if !yield(measurement) {
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
) iter.Seq[*data.Measurement[float64]] {
	return catalog.SignalLogic(ctx, epoch, label, lowTick, highTick, sources...)
}

/*
Timeline reconstructs the sequential market tape from the unified Measurements table.
*/
func (catalog *Catalog) Timeline(
	ctx context.Context,
	epoch int64,
	label string,
	fromTick int64,
	toTick int64,
) iter.Seq[*data.Measurement[float64]] {
	var filter iceberg.BooleanExpression

	if label != "" {
		filter = iceberg.EqualTo(iceberg.Reference("label"), label)
	}

	if fromTick > 0 {
		expression := iceberg.NewOr(
			iceberg.GreaterThanEqual(iceberg.Reference("tick"), fromTick),
			iceberg.GreaterThanEqual(iceberg.Reference("seqIdx"), fromTick),
		)

		if filter != nil {
			expression = iceberg.NewAnd(filter, expression)
		}

		filter = expression
	}

	if toTick > 0 && toTick >= fromTick {
		expression := iceberg.NewOr(
			iceberg.LessThanEqual(iceberg.Reference("tick"), toTick),
			iceberg.LessThanEqual(iceberg.Reference("seqIdx"), toTick),
		)

		if filter != nil {
			expression = iceberg.NewAnd(filter, expression)
		}

		filter = expression
	}

	return catalog.Scan(ctx, Measurements, epoch, filter, 0)
}

/*
Labels discovers distinct labels present in an epoch by reading only the label column.
*/
func (catalog *Catalog) Labels(ctx context.Context, epoch int64) ([]string, error) {
	labelSet := make(map[string]struct{})

	for measurement := range catalog.Scan(ctx, Measurements, epoch, nil, 0, "label") {
		if measurement.Label != "" {
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
