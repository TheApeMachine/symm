package strategy

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"iter"
	"unsafe"

	"github.com/apache/arrow-go/v18/arrow/ipc"
	"github.com/theapemachine/symm/hindsight/tables"
	"github.com/theapemachine/symm/workbench"
)

/*
Excursions uses DuckDB's analytical engine to execute a vectorized SQL query
over the Iceberg warehouse. It converts the returned Apache Arrow IPC stream
back into measurements and yields them.

This isolates complex excursion-finding logic (e.g. window functions) from
the training pipeline, keeping it a clean streaming primitive.
*/
type Excursions struct {
	ctx       context.Context
	warehouse *workbench.Warehouse
	query     string
	err       error
}

func NewExcursions(ctx context.Context, warehouse *workbench.Warehouse, extType int) *Excursions {
	// Formulate a sophisticated SQL window query to find genuine market excursions.
	// We extract a 100,000 tick window from recent history, and use DuckDB's native
	// window functions to categorize sustained movements into the 5 types from TRAINING.md.
	
	// Map the extType to the SQL condition
	var condition string
	switch extType {
	case 1: // Upwards movement, clearing friction (profitable)
		condition = "peak_price > start_price * 1.002 AND peak_tick > start_tick + 500"
	case 2: // Upwards movement, not clearing friction (unprofitable)
		condition = "peak_price > start_price * 1.0005 AND peak_price <= start_price * 1.002 AND peak_tick > start_tick + 500"
	case 3: // Downwards movement (unprofitable)
		condition = "bottom_price < start_price * 0.998 AND bottom_tick > start_tick + 500"
	case 5: // Flat line (unprofitable)
		condition = "peak_price < start_price * 1.0005 AND bottom_price > start_price * 0.9995 AND peak_tick > start_tick + 500"
	default: // 4. Choppy/sideways movement (unprofitable)
		condition = "peak_price >= start_price * 1.0005 AND peak_price <= start_price * 1.002 AND bottom_price <= start_price * 0.9995 AND peak_tick < start_tick + 500"
	}
	
	query := fmt.Sprintf(`
		WITH recent_tape AS (
			SELECT *,
			       list_extract(map_extract(metrics, 'best_bid'), 1).raw AS price
			FROM %s.measurements
			WHERE metrics IS NOT NULL
			ORDER BY tick DESC
			LIMIT 100000
		),
		pump_windows AS (
			SELECT 
				symbol,
				tick AS start_tick,
				price AS start_price,
				arg_max(tick, price) OVER w AS peak_tick,
				max(price) OVER w AS peak_price,
				arg_min(tick, price) OVER w AS bottom_tick,
				min(price) OVER w AS bottom_price
			FROM recent_tape
			WHERE price IS NOT NULL
			-- Look ahead 10,000 ticks to analyze the development of the precursor
			WINDOW w AS (PARTITION BY symbol ORDER BY tick ASC ROWS BETWEEN CURRENT ROW AND 10000 FOLLOWING)
		),
		valid_pumps AS (
			SELECT * FROM pump_windows
			WHERE %s
			ORDER BY random()
			LIMIT 1
		)
		SELECT m.* 
		FROM %s.measurements m
		JOIN valid_pumps p ON m.symbol = p.symbol
		-- Extract the exact time fragment, bounding from the start to the peak/bottom
		WHERE m.tick BETWEEN p.start_tick AND GREATEST(p.peak_tick, p.bottom_tick)
		ORDER BY m.tick ASC
	`, tables.Namespace, condition, tables.Namespace)

	return &Excursions{
		ctx:       ctx,
		warehouse: warehouse,
		query:     query,
	}
}

func (op *Excursions) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		stream, err := op.warehouse.Execute(op.ctx, op.query)
		if err != nil {
			op.Error(err)
			return
		}

		if len(stream) == 0 {
			return
		}

		reader, err := ipc.NewReader(bytes.NewReader(stream))
		if err != nil {
			op.Error(err)
			return
		}
		defer reader.Release()

		for reader.Next() {
			batch := reader.RecordBatch()
			measurements, err := tables.ReadMeasurements(batch)
			if err != nil {
				op.Error(err)
				return
			}

			for _, m := range measurements {
				if m != nil {
					if !yield(unsafe.Pointer(&m)) {
						return
					}
				}
			}
		}

		if err := reader.Err(); err != nil {
			op.Error(err)
		}
	}
}

func (op *Excursions) Error(errs ...error) error {
	for _, err := range errs {
		if err != nil {
			op.err = errors.Join(op.err, err)
		}
	}
	return op.err
}
