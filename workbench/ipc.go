package workbench

import (
	"bytes"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/apache/arrow-go/v18/arrow"
	"github.com/apache/arrow-go/v18/arrow/array"
	"github.com/apache/arrow-go/v18/arrow/ipc"
	"github.com/apache/arrow-go/v18/arrow/memory"
)

/*
Column is one column of a statement's result.
*/
type Column struct {
	Name string
	Type string
}

func encodeIPC(columns []Column, rows [][]any) ([]byte, error) {
	if len(columns) == 0 {
		return nil, fmt.Errorf("workbench: no columns to encode")
	}

	fields := make([]arrow.Field, 0, len(columns))
	for _, column := range columns {
		fields = append(fields, arrow.Field{
			Name:     column.Name,
			Type:     arrowType(column.Type),
			Nullable: true,
		})
	}

	schema := arrow.NewSchema(fields, nil)
	pool := memory.NewGoAllocator()
	builders := make([]array.Builder, len(columns))

	for i, field := range fields {
		builders[i] = array.NewBuilder(pool, field.Type)
	}

	defer func() {
		for _, builder := range builders {
			builder.Release()
		}
	}()

	for _, row := range rows {
		for i, value := range row {
			appendValue(builders[i], value)
		}
	}

	columnsOut := make([]arrow.Array, len(builders))
	for i, builder := range builders {
		columnsOut[i] = builder.NewArray()
	}

	defer func() {
		for _, column := range columnsOut {
			column.Release()
		}
	}()

	record := array.NewRecord(schema, columnsOut, int64(len(rows)))
	defer record.Release()

	var buf bytes.Buffer
	writer := ipc.NewWriter(&buf, ipc.WithSchema(schema))

	if err := writer.Write(record); err != nil {
		_ = writer.Close()
		return nil, err
	}

	if err := writer.Close(); err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}

func arrowType(kind string) arrow.DataType {
	normalized := strings.ToUpper(strings.TrimSpace(kind))

	switch {
	case strings.HasPrefix(normalized, "TINYINT"), normalized == "INT1":
		return arrow.PrimitiveTypes.Int8
	case strings.HasPrefix(normalized, "SMALLINT"), normalized == "INT2":
		return arrow.PrimitiveTypes.Int16
	case strings.HasPrefix(normalized, "INTEGER"), normalized == "INT", normalized == "INT4":
		return arrow.PrimitiveTypes.Int32
	case strings.HasPrefix(normalized, "BIGINT"), normalized == "INT8",
		strings.HasPrefix(normalized, "HUGEINT"),
		strings.HasPrefix(normalized, "UBIGINT"),
		strings.HasPrefix(normalized, "UINTEGER"):
		return arrow.PrimitiveTypes.Int64
	case strings.HasPrefix(normalized, "FLOAT"), normalized == "FLOAT4",
		strings.HasPrefix(normalized, "REAL"):
		return arrow.PrimitiveTypes.Float32
	case strings.HasPrefix(normalized, "DOUBLE"), normalized == "FLOAT8",
		strings.HasPrefix(normalized, "DECIMAL"),
		strings.HasPrefix(normalized, "NUMERIC"):
		return arrow.PrimitiveTypes.Float64
	case strings.HasPrefix(normalized, "BOOLEAN"), normalized == "BOOL":
		return arrow.FixedWidthTypes.Boolean
	case strings.HasPrefix(normalized, "TIMESTAMP"),
		strings.HasPrefix(normalized, "DATETIME"):
		return arrow.FixedWidthTypes.Timestamp_us
	case strings.HasPrefix(normalized, "DATE"):
		return arrow.FixedWidthTypes.Date32
	default:
		return arrow.BinaryTypes.String
	}
}

func appendValue(builder array.Builder, value any) {
	if value == nil {
		builder.AppendNull()
		return
	}

	switch b := builder.(type) {
	case *array.BooleanBuilder:
		b.Append(asBool(value))
	case *array.Int8Builder:
		b.Append(int8(asInt64(value)))
	case *array.Int16Builder:
		b.Append(int16(asInt64(value)))
	case *array.Int32Builder:
		b.Append(int32(asInt64(value)))
	case *array.Int64Builder:
		b.Append(asInt64(value))
	case *array.Float32Builder:
		b.Append(float32(asFloat64(value)))
	case *array.Float64Builder:
		b.Append(asFloat64(value))
	case *array.StringBuilder:
		b.Append(asString(value))
	case *array.TimestampBuilder:
		if ts, ok := asTimestamp(value); ok {
			b.Append(arrow.Timestamp(ts.UnixMicro()))
			return
		}
		b.AppendNull()
	case *array.Date32Builder:
		if ts, ok := asTimestamp(value); ok {
			b.Append(arrow.Date32(ts.UTC().Unix() / 86400))
			return
		}
		b.AppendNull()
	default:
		builder.AppendNull()
	}
}

func asBool(value any) bool {
	switch v := value.(type) {
	case bool:
		return v
	case int64:
		return v != 0
	case string:
		parsed, _ := strconv.ParseBool(v)
		return parsed
	default:
		return false
	}
}

func asInt64(value any) int64 {
	switch v := value.(type) {
	case int64:
		return v
	case int32:
		return int64(v)
	case int16:
		return int64(v)
	case int8:
		return int64(v)
	case int:
		return int64(v)
	case uint:
		if uint64(v) > math.MaxInt64 {
			return math.MaxInt64
		}
		return int64(v)
	case uint8:
		return int64(v)
	case uint16:
		return int64(v)
	case uint32:
		return int64(v)
	case uint64:
		if v > math.MaxInt64 {
			return math.MaxInt64
		}
		return int64(v)
	case float64:
		return int64(v)
	case float32:
		return int64(v)
	case string:
		parsed, _ := strconv.ParseInt(v, 10, 64)
		return parsed
	case []byte:
		parsed, _ := strconv.ParseInt(string(v), 10, 64)
		return parsed
	default:
		return 0
	}
}

func asFloat64(value any) float64 {
	switch v := value.(type) {
	case float64:
		return v
	case float32:
		return float64(v)
	case int64:
		return float64(v)
	case int32:
		return float64(v)
	case int:
		return float64(v)
	case string:
		parsed, _ := strconv.ParseFloat(v, 64)
		return parsed
	case []byte:
		parsed, _ := strconv.ParseFloat(string(v), 64)
		return parsed
	default:
		return 0
	}
}

func asString(value any) string {
	switch v := value.(type) {
	case string:
		return v
	case []byte:
		return string(v)
	case time.Time:
		return v.UTC().Format(time.RFC3339Nano)
	default:
		return fmt.Sprint(v)
	}
}

func asTimestamp(value any) (time.Time, bool) {
	switch v := value.(type) {
	case time.Time:
		return v, true
	case string:
		for _, layout := range []string{
			time.RFC3339Nano,
			time.RFC3339,
			"2006-01-02 15:04:05.999999-07:00",
			"2006-01-02 15:04:05.999999",
			"2006-01-02",
		} {
			if parsed, err := time.Parse(layout, v); err == nil {
				return parsed, true
			}
		}
	}
	return time.Time{}, false
}
