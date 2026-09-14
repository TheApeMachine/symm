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
