package tables

import (
	"context"
	"iter"

	"github.com/apache/iceberg-go"
	icetable "github.com/apache/iceberg-go/table"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/data"
)

/*
Scan reads measurements from a canonical table matching the epoch and filter predicates,
projecting only the requested columns and stopping early if limit is reached.
*/
func (catalog *Catalog) scan(
	ctx context.Context,
	tableName string,
	epoch int64,
	filter iceberg.BooleanExpression,
	limit int,
	fields ...string,
) iter.Seq2[*data.Measurement[float64], error] {
	return func(yield func(*data.Measurement[float64], error) bool) {
		tbl, err := catalog.Load(ctx, tableName)

		if err != nil {
			yield(nil, errnie.Error(errnie.Err(
				errnie.NotFound,
				"[catalog] unable to find: "+tableName,
				err,
			)))

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
			yield(nil, errnie.Error(errnie.Err(
				errnie.BadGateway,
				"[iceberg] failed to plan files for "+tableName,
				err,
			)))

			return
		}

		_, batches, err := tbl.Scan(options...).ReadTasks(ctx, tasks)

		if err != nil {
			yield(nil, errnie.Error(errnie.Err(
				errnie.BadGateway,
				"[iceberg] failed to read tasks for "+tableName,
				err,
			)))

			return
		}

		var count int

		for batch, batchErr := range batches {
			if batchErr != nil {
				yield(nil, errnie.Error(errnie.Err(
					errnie.BadGateway,
					"[iceberg] batch decode failure for "+tableName,
					batchErr,
				)))

				return
			}

			if batch != nil {
				batchMeasurements, err := readMeasurements(batch)
				batch.Release()

				if err != nil {
					yield(nil, err)
					return
				}

				for _, measurement := range batchMeasurements {
					if !yield(measurement, nil) {
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

// Scan exposes the existing display scan; replay uses scan directly to propagate failures.
func (catalog *Catalog) Scan(ctx context.Context, tableName string, epoch int64,
	filter iceberg.BooleanExpression, limit int, fields ...string,
) iter.Seq[*data.Measurement[float64]] {
	return func(yield func(*data.Measurement[float64]) bool) {
		for measurement, err := range catalog.scan(ctx, tableName, epoch, filter, limit, fields...) {
			if err != nil {
				errnie.Error(err)
				return
			}
			if !yield(measurement) {
				return
			}
		}
	}
}
