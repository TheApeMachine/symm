package tables

import (
	"math"
	"time"

	"github.com/apache/arrow-go/v18/arrow"
	"github.com/apache/arrow-go/v18/arrow/array"
	"github.com/apache/arrow-go/v18/arrow/memory"
	"github.com/apache/iceberg-go"
	icetable "github.com/apache/iceberg-go/table"
	"github.com/theapemachine/errnie"
)

/*
arrowSchemaFor converts an Iceberg schema to the Arrow schema an append
expects.

Field IDs must be carried in the Arrow metadata: the writer matches columns to
the table by ID, not by name, so a schema built without them produces files the
table cannot read back. The schema this is given must be the loaded table's
own, never a local literal — a catalog assigns its own field IDs when it
creates a table, and a record built from the literal has IDs that disagree with
the table's, which surfaces as a column resolved against an entirely different
one.
*/
func arrowSchemaFor(schema *iceberg.Schema) (*arrow.Schema, error) {
	converted, err := icetable.SchemaToArrowSchema(schema, nil, true, false)

	if err != nil {
		return nil, errnie.Error(errnie.Err(
			errnie.Internal,
			"[iceberg] failed to convert schema to arrow",
			err,
		))
	}

	return converted, nil
}

/*
span is how many rows the next snapshot carries.

Every append is a snapshot plus a metadata write, so a commit that carries too
many bytes runs past the catalog's HTTP and object-store timeouts. The rows are
cut into spans that fit instead, and a single row too large to encode at all is
refused rather than truncated.
*/
func span(start, count, limit int, payloadSize func(int) int) (int, int, error) {
	ceiling := math.MaxInt32

	if limit > 0 && limit < ceiling {
		ceiling = limit
	}

	end, size := start, 0

	for end < count {
		length := 0

		if payloadSize != nil {
			length = payloadSize(end)
		}

		if length > math.MaxInt32 {
			return start, 0, errnie.Error(errnie.Err(
				errnie.Validation,
				"[iceberg] one payload exceeds Arrow Binary's signed 32-bit offset limit",
				nil,
			))
		}

		if size > 0 && length > ceiling-size {
			break
		}

		size += length
		end++
	}

	return end, size, nil
}

/*
records builds the Arrow batches one append sends.
*/
func records(
	schema *iceberg.Schema, count int, payloadSize func(int) int,
	fill func(*array.RecordBuilder, int, int),
) (array.RecordReader, error) {
	converted, err := arrowSchemaFor(schema)

	if err != nil {
		return nil, err
	}

	builder := array.NewRecordBuilder(memory.DefaultAllocator, converted)
	defer builder.Release()

	batches := []arrow.RecordBatch{}

	defer func() {
		for _, batch := range batches {
			batch.Release()
		}
	}()

	for start := 0; start < count; {
		end, _, err := span(start, count, math.MaxInt32, payloadSize)

		if err != nil {
			return nil, err
		}

		builder.Reserve(end - start)
		fill(builder, start, end)
		batches = append(batches, builder.NewRecord())
		start = end
	}

	reader, err := array.NewRecordReader(converted, batches)

	if err != nil {
		return nil, errnie.Error(errnie.Err(
			errnie.Internal,
			"[iceberg] failed to build record reader",
			err,
		))
	}

	return reader, nil
}

/*
timestamp encodes a Go time as Iceberg's microsecond-resolution timestamptz.
Nanosecond precision is discarded here, which is real for anything reading the
column back as an instant.
*/
func timestamp(builder *array.TimestampBuilder, value time.Time) {
	if value.IsZero() {
		builder.AppendNull()
		return
	}

	builder.Append(arrow.Timestamp(value.UTC().UnixMicro()))
}
