package tables

import "github.com/apache/iceberg-go"

const (
	Namespace    = "hindsight"
	Measurements = "measurements"
	Runs         = "runs"
)

/*
MeasurementSchema defines the canonical tabular representation of *data.Measurement[float64].
Top-level columns epoch, source, and label allow Iceberg partition pruning and file-level
min/max metric pruning before Parquet pages are fetched.
*/
func MeasurementSchema() *iceberg.Schema {
	return iceberg.NewSchema(0,
		iceberg.NestedField{ID: 1, Name: "epoch", Type: iceberg.PrimitiveTypes.Int64, Required: true},
		iceberg.NestedField{ID: 2, Name: "seqIdx", Type: iceberg.PrimitiveTypes.Int64, Required: true},
		iceberg.NestedField{ID: 3, Name: "source", Type: iceberg.PrimitiveTypes.String, Required: true},
		iceberg.NestedField{ID: 4, Name: "label", Type: iceberg.PrimitiveTypes.String, Required: true},
		iceberg.NestedField{ID: 5, Name: "at", Type: iceberg.PrimitiveTypes.TimestampTz, Required: true},
		iceberg.NestedField{ID: 6, Name: "maturity", Type: iceberg.PrimitiveTypes.Float64, Required: true},
		iceberg.NestedField{ID: 7, Name: "snr", Type: iceberg.PrimitiveTypes.Float64, Required: false},
		iceberg.NestedField{ID: 8, Name: "snrDefined", Type: iceberg.PrimitiveTypes.Bool, Required: true},
		iceberg.NestedField{ID: 9, Name: "metrics", Required: false,
			Type: &iceberg.MapType{
				KeyID: 101, KeyType: iceberg.PrimitiveTypes.String,
				ValueID: 102, ValueType: iceberg.PrimitiveTypes.Float64, ValueRequired: false,
			},
		},
		iceberg.NestedField{ID: 10, Name: "metadata", Required: false,
			Type: &iceberg.MapType{
				KeyID: 201, KeyType: iceberg.PrimitiveTypes.String,
				ValueID: 202, ValueType: iceberg.PrimitiveTypes.String, ValueRequired: false,
			},
		},
		iceberg.NestedField{ID: 11, Name: "provenance", Required: false,
			Type: &iceberg.MapType{
				KeyID: 301, KeyType: iceberg.PrimitiveTypes.String,
				ValueID: 302, ValueType: iceberg.PrimitiveTypes.String, ValueRequired: false,
			},
		},
	)
}

/*
MeasurementPartitioning isolates runs physically by identity partitioning on epoch, source, and label.
*/
func MeasurementPartitioning() iceberg.PartitionSpec {
	return iceberg.NewPartitionSpec(
		iceberg.PartitionField{SourceIDs: []int{1}, FieldID: 1000, Name: "epoch", Transform: iceberg.IdentityTransform{}},
		iceberg.PartitionField{SourceIDs: []int{3}, FieldID: 1001, Name: "source", Transform: iceberg.IdentityTransform{}},
		iceberg.PartitionField{SourceIDs: []int{4}, FieldID: 1002, Name: "label", Transform: iceberg.IdentityTransform{}},
	)
}

/*
RunsSchema defines the canonical metadata table storing process run facts.
*/
func RunsSchema() *iceberg.Schema {
	return iceberg.NewSchema(0,
		iceberg.NestedField{ID: 1, Name: "epoch", Type: iceberg.PrimitiveTypes.Int64, Required: true},
		iceberg.NestedField{ID: 2, Name: "started_at", Type: iceberg.PrimitiveTypes.TimestampTz, Required: true},
		iceberg.NestedField{ID: 3, Name: "code_commit", Type: iceberg.PrimitiveTypes.String, Required: true},
		iceberg.NestedField{ID: 4, Name: "build_id", Type: iceberg.PrimitiveTypes.String, Required: true},
		iceberg.NestedField{ID: 5, Name: "config_digest", Type: iceberg.PrimitiveTypes.String, Required: true},
		iceberg.NestedField{ID: 6, Name: "status", Type: iceberg.PrimitiveTypes.String, Required: true},
	)
}

/*
RunsPartitioning leaves the small runs metadata table unpartitioned.
*/
func RunsPartitioning() iceberg.PartitionSpec {
	return iceberg.NewPartitionSpec()
}
