package tables

import "github.com/apache/iceberg-go"

const (
	Namespace    = "hindsight"
	Measurements = "measurements"
	Runs         = "runs"
	// Detections holds source=detector excursion labels apart from the
	// observation tape, with the measurement schema and partitioning.
	Detections = "detections"
)

/*
MeasurementSchema defines the canonical tabular representation of *data.Measurement.
Top-level columns epoch, source, and label allow Iceberg partition pruning and file-level
min/max metric pruning before Parquet pages are fetched.
*/
func MeasurementSchema() *iceberg.Schema {
	metricStruct := &iceberg.StructType{
		FieldList: []iceberg.NestedField{
			{ID: 18, Name: "raw", Type: iceberg.PrimitiveTypes.Float64, Required: true},
			{ID: 19, Name: "normalized", Type: iceberg.PrimitiveTypes.Float64, Required: true},
			{ID: 20, Name: "standardized", Type: iceberg.PrimitiveTypes.Float64, Required: true},
			{ID: 21, Name: "exact", Type: iceberg.PrimitiveTypes.String, Required: false},
			{ID: 22, Name: "center", Type: iceberg.PrimitiveTypes.Float64, Required: true},
			{ID: 23, Name: "scale", Type: iceberg.PrimitiveTypes.Float64, Required: true},
			{ID: 24, Name: "unit", Type: iceberg.PrimitiveTypes.String, Required: true},
			{ID: 25, Name: "timescale", Type: iceberg.PrimitiveTypes.String, Required: true},
		},
	}

	return iceberg.NewSchema(0,
		iceberg.NestedField{ID: 1, Name: "epoch", Type: iceberg.PrimitiveTypes.Int64, Required: true},
		iceberg.NestedField{ID: 2, Name: "seqIdx", Type: iceberg.PrimitiveTypes.Int64, Required: true},
		iceberg.NestedField{ID: 3, Name: "source", Type: iceberg.PrimitiveTypes.String, Required: true},
		iceberg.NestedField{ID: 4, Name: "label", Type: iceberg.PrimitiveTypes.String, Required: true},
		iceberg.NestedField{ID: 5, Name: "tick", Type: iceberg.PrimitiveTypes.Int64, Required: true},
		iceberg.NestedField{ID: 6, Name: "at", Type: iceberg.PrimitiveTypes.TimestampTz, Required: true},
		iceberg.NestedField{ID: 7, Name: "from", Type: iceberg.PrimitiveTypes.TimestampTz, Required: true},
		iceberg.NestedField{ID: 8, Name: "timestamp", Type: iceberg.PrimitiveTypes.Int64, Required: true},
		iceberg.NestedField{ID: 9, Name: "id", Type: iceberg.PrimitiveTypes.Int64, Required: true},
		iceberg.NestedField{ID: 10, Name: "coherence", Type: iceberg.PrimitiveTypes.Float64, Required: true},
		iceberg.NestedField{ID: 11, Name: "maturity", Type: iceberg.PrimitiveTypes.Float64, Required: true},
		iceberg.NestedField{ID: 12, Name: "samples", Type: iceberg.PrimitiveTypes.Int64, Required: true},
		iceberg.NestedField{ID: 13, Name: "energy", Type: iceberg.PrimitiveTypes.Float64, Required: true},
		iceberg.NestedField{ID: 14, Name: "prediction", Type: iceberg.PrimitiveTypes.Float64, Required: true},
		iceberg.NestedField{ID: 15, Name: "metrics", Required: false,
			Type: &iceberg.MapType{
				KeyID: 16, KeyType: iceberg.PrimitiveTypes.String,
				ValueID: 17, ValueType: metricStruct, ValueRequired: false,
			},
		},
		iceberg.NestedField{ID: 26, Name: "metadata", Required: false,
			Type: &iceberg.MapType{
				KeyID: 27, KeyType: iceberg.PrimitiveTypes.String,
				ValueID: 28, ValueType: iceberg.PrimitiveTypes.String, ValueRequired: false,
			},
		},
	)
}

/*
MeasurementPartitioning isolates runs physically by identity partitioning on epoch and source.
*/
func MeasurementPartitioning() iceberg.PartitionSpec {
	return iceberg.NewPartitionSpec(
		iceberg.PartitionField{
			SourceIDs: []int{1},
			FieldID:   1000,
			Name:      "epoch",
			Transform: iceberg.IdentityTransform{},
		},
		iceberg.PartitionField{
			SourceIDs: []int{3},
			FieldID:   1001,
			Name:      "source",
			Transform: iceberg.IdentityTransform{},
		},
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
