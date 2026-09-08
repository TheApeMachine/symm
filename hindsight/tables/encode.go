package tables

import (
	"math/big"
	"time"

	"github.com/apache/arrow-go/v18/arrow"
	"github.com/apache/arrow-go/v18/arrow/array"
	"github.com/apache/arrow-go/v18/arrow/decimal128"
	"github.com/apache/arrow-go/v18/arrow/memory"
	"github.com/apache/iceberg-go"
	icetable "github.com/apache/iceberg-go/table"
	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/theapemachine/errnie"
)

/*
arrowSchemaFor converts an Iceberg schema to the Arrow schema an append
expects. Field IDs must be carried in the Arrow metadata: the writer matches
columns to the table by ID, not by name, so a schema built without them
produces files the table cannot read back.
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
timestamp encodes a Go time as Iceberg's microsecond-resolution timestamptz.

Nanosecond precision is discarded here. Capture ordering never depends on it —
CaptureSequence is the ordering, not the clock — but the truncation is real for
anything reading received_at as an instant.
*/
func timestamp(builder *array.TimestampBuilder, value time.Time) {
	if value.IsZero() {
		builder.AppendNull()

		return
	}

	builder.Append(arrow.Timestamp(value.UTC().UnixMicro()))
}

/*
money encodes a Kraken decimal into a fixed-scale Iceberg decimal.

Both sides are an unscaled integer plus a scale, so the conversion is a
rescale of the unscaled value rather than a parse or a float round-trip. A
value carrying more than DecimalScale fractional digits would lose the excess;
the venue quotes far fewer, and the rescale below only ever multiplies.
*/
func money(builder *array.Decimal128Builder, value *decimal.Decimal) {
	if value == nil {
		builder.AppendNull()

		return
	}

	unscaled := new(big.Int).Set(value.RawBigInt())
	shift := DecimalScale - value.GetScale()

	if shift < 0 {
		// Scale exceeds the column's. Divide rather than silently emit a value
		// that is 10^-shift times too large.
		unscaled.Quo(unscaled, new(big.Int).Exp(big.NewInt(10), big.NewInt(-shift), nil))
	}

	if shift > 0 {
		unscaled.Mul(unscaled, new(big.Int).Exp(big.NewInt(10), big.NewInt(shift), nil))
	}

	builder.Append(decimal128.FromBigInt(unscaled))
}

// text appends a string, encoding the empty string as null so that optional
// columns stay genuinely absent rather than holding a sentinel.
func text(builder *array.StringBuilder, value string) {
	if value == "" {
		builder.AppendNull()

		return
	}

	builder.Append(value)
}

// envelope fills one nested envelope reference.
func envelope(builder *array.StructBuilder, ref EnvelopeRefRow) {
	builder.Append(true)
	builder.FieldBuilder(0).(*array.StringBuilder).Append(ref.Run)
	builder.FieldBuilder(1).(*array.Int64Builder).Append(ref.Sequence)
	builder.FieldBuilder(2).(*array.Int64Builder).Append(ref.Ordinal)
}

/*
records builds a single Arrow record for one table from n rows.

Every append path is the same shape: convert the Iceberg schema, drive a
RecordBuilder through fill, and hand the result back as a one-record reader.
Batching is the caller's concern — Writer accumulates rows so that one commit
covers many records rather than one snapshot per row.
*/
func records(
	schema *iceberg.Schema, count int, fill func(*array.RecordBuilder),
) (array.RecordReader, error) {
	converted, err := arrowSchemaFor(schema)

	if err != nil {
		return nil, err
	}

	builder := array.NewRecordBuilder(memory.DefaultAllocator, converted)
	defer builder.Release()

	builder.Reserve(count)
	fill(builder)

	record := builder.NewRecord()
	defer record.Release()

	reader, err := array.NewRecordReader(converted, []arrow.RecordBatch{record})

	if err != nil {
		return nil, errnie.Error(errnie.Err(
			errnie.Internal,
			"[iceberg] failed to build record reader",
			err,
		))
	}

	return reader, nil
}
