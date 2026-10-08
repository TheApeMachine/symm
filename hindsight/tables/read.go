package tables

import (
	"context"
	"iter"
	"sort"

	"github.com/apache/iceberg-go"
	icetable "github.com/apache/iceberg-go/table"
	"github.com/apache/iceberg-go/utils"
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
) iter.Seq2[*data.Measurement, error] {
	return func(yield func(*data.Measurement, error) bool) {
		if catalog != nil && catalog.awsConfig != nil {
			ctx = utils.WithAwsConfig(ctx, catalog.awsConfig)
		}

		tbl, err := catalog.Load(ctx, tableName)

		if err != nil {
			yield(nil, errnie.Error(errnie.Err(
				errnie.NotFound,
				"[catalog] unable to find: "+tableName,
				err,
			)))

			return
		}

		var predicate iceberg.BooleanExpression

		if epoch > 0 {
			predicate = iceberg.EqualTo(iceberg.Reference("epoch"), epoch)
		}

		if filter != nil && predicate != nil {
			predicate = iceberg.NewAnd(predicate, filter)
		}

		if filter != nil && predicate == nil {
			predicate = filter
		}

		options := []icetable.ScanOption{icetable.WitMaxConcurrency(32)}

		if predicate != nil {
			options = append(options, icetable.WithRowFilter(predicate))
		}

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
				batchMeasurements, err := ReadMeasurements(batch)
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

// Collect reads every measurement for one epoch. The scan predicate is the epoch.
func (catalog *Catalog) Collect(ctx context.Context, tableName string, epoch int64) ([]*data.Measurement, error) {
	if catalog == nil {
		return nil, errnie.Error(errnie.Err(
			errnie.Validation,
			"catalog: catalog is required",
			nil,
		))
	}

	rows := make([]*data.Measurement, 0)

	for measurement, err := range catalog.scan(ctx, tableName, epoch, nil, 0) {
		if err != nil {
			return nil, err
		}

		if measurement != nil {
			rows = append(rows, measurement)
		}
	}

	return rows, nil
}

/*
Scan reads measurements from a table for display and replay consumers, sorted by
SeqIdx (then Tick) so Timeline merge (which assumes ordered streams) is correct.

A read failure is yielded as a non-nil error and ends the sequence; it is never
reported as end-of-stream. Because rows are sorted before they are yielded, a
failure yields no rows at all: a partial window never looks like a short one.
*/
func (catalog *Catalog) Scan(ctx context.Context, tableName string, epoch int64,
	filter iceberg.BooleanExpression, limit int, fields ...string,
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

		rows := make([]*data.Measurement, 0)

		for measurement, err := range catalog.scan(ctx, tableName, epoch, filter, limit, fields...) {
			if err != nil {
				yield(nil, err)
				return
			}

			if measurement != nil {
				rows = append(rows, measurement)
			}
		}

		sort.SliceStable(rows, func(i, j int) bool {
			if rows[i].SeqIdx != rows[j].SeqIdx {
				return rows[i].SeqIdx < rows[j].SeqIdx
			}

			return rows[i].Tick < rows[j].Tick
		})

		for _, measurement := range rows {
			if !yield(measurement, nil) {
				return
			}
		}
	}
}
