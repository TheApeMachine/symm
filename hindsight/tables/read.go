package tables

import (
	"context"
	"iter"
	"unsafe"

	"github.com/apache/iceberg-go"
	icetable "github.com/apache/iceberg-go/table"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/data"
)

/*
Scan reads measurements from a canonical table matching the epoch and filter predicates,
projecting only the requested columns and stopping early if limit is reached.
*/
func (catalog *Catalog) Scan(
	ctx context.Context,
	tableName string,
	epoch int64,
	filter iceberg.BooleanExpression,
	limit int,
	fields ...string,
) iter.Seq[*data.Measurement[float64]] {
	return func(yield func(*data.Measurement[float64]) bool) {
		tbl, err := catalog.Load(ctx, tableName)

		if err != nil {
			errnie.Error(errnie.Err(
				errnie.NotFound,
				"[catalog] unable to find: "+tableName,
				err,
			))

			return
		}

		predicate := iceberg.BooleanExpression(iceberg.EqualTo(iceberg.Reference("epoch"), epoch))

		if filter != nil {
			predicate = iceberg.NewAnd(predicate, filter)
		}

		options := []icetable.ScanOption{icetable.WithRowFilter(predicate)}

		if len(fields) > 0 {
			options = append(options, icetable.WithSelectedFields(fields...))
		}

		tasks, err := tbl.Scan(options...).PlanFiles(ctx)

		if err != nil {
			errnie.Error(errnie.Err(
				errnie.BadGateway,
				"[iceberg] failed to plan files for "+tableName,
				err,
			))

			return
		}

		_, batches, err := tbl.Scan(options...).ReadTasks(ctx, tasks)

		if err != nil {
			errnie.Error(errnie.Err(
				errnie.BadGateway,
				"[iceberg] failed to read tasks for "+tableName,
				err,
			))

			return
		}

		var count int

		for batch, batchErr := range batches {
			if batchErr != nil {
				errnie.Error(errnie.Err(
					errnie.BadGateway,
					"[iceberg] batch decode failure for "+tableName,
					batchErr,
				))

				return
			}

			if batch != nil {
				batchMeasurements := readMeasurements(batch)
				batch.Release()

				for _, measurement := range batchMeasurements {
					if !yield(measurement) {
						return
					}

					count++

					if limit > 0 && count >= limit {
						return
					}
				}
			}
		}
	}
}

/*
Replay loads all excursions for the given epoch and yields them alongside their
accompanying measurements presented as an iter.Seq[iter.Seq[unsafe.Pointer]].
Each inner sequence yields unsafe.Pointer(*data.Measurement[float64]).
*/
func (catalog *Catalog) Replay(
	ctx context.Context,
	epoch int64,
) ([]ExcursionRecord, iter.Seq[iter.Seq[unsafe.Pointer]], error) {
	excursions, err := catalog.Excursions(ctx, epoch, nil)

	if err != nil {
		return nil, nil, errnie.Error(errnie.Err(
			errnie.BadGateway,
			"[catalog] failed to load excursions for replay",
			err,
		))
	}

	fragments := func(yield func(iter.Seq[unsafe.Pointer]) bool) {
		for _, excursion := range excursions {
			predicate := iceberg.BooleanExpression(
				iceberg.EqualTo(iceberg.Reference("symbol"), excursion.Symbol),
			)

			endTick := excursion.PostEndTick

			if endTick <= 0 {
				endTick = excursion.ExitTick
			}

			if endTick > 0 && excursion.PrecursorStartTick >= 0 {
				predicate = iceberg.NewAnd(
					predicate,
					iceberg.NewAnd(
						iceberg.GreaterThanEqual(iceberg.Reference("tick"), excursion.PrecursorStartTick),
						iceberg.LessThanEqual(iceberg.Reference("tick"), endTick),
					),
				)
			}

			measurementsSeq := catalog.Scan(ctx, Measurements, epoch, predicate, 0)

			inner := func(innerYield func(unsafe.Pointer) bool) {
				for measurement := range measurementsSeq {
					if !innerYield(unsafe.Pointer(measurement)) {
						return
					}
				}
			}

			if !yield(inner) {
				return
			}
		}
	}

	return excursions, fragments, nil
}

