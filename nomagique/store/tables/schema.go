package tables

import (
	"strings"

	"github.com/apache/iceberg-go"
	"github.com/bytedance/sonic"
	"github.com/theapemachine/errnie"
)

/*
TableFieldConfig declares a single field's metadata in an Iceberg schema.
*/
type TableFieldConfig struct {
	ID       int    `json:"id"`
	Name     string `json:"name"`
	Type     string `json:"type"`
	Required bool   `json:"required"`
}

/*
TableConfig declares the table identifier, catalog endpoints, fields, and partition spec.
Config comes entirely from JSON.
*/
type TableConfig struct {
	Namespace  string             `json:"namespace"`
	Table      string             `json:"table"`
	URI        string             `json:"uri"`
	Warehouse  string             `json:"warehouse"`
	Fields     []TableFieldConfig `json:"fields"`
	Partitions []string           `json:"partitions"`
}

/*
SchemaFromJSON builds an Iceberg schema dynamically from a JSON configuration string.
*/
func SchemaFromJSON(jsonConfig string) (*iceberg.Schema, error) {
	var cfg TableConfig
	if err := sonic.Unmarshal([]byte(jsonConfig), &cfg); err != nil {
		return nil, errnie.Error(errnie.Err(errnie.Validation, "tables: invalid table schema JSON", err))
	}

	fields := make([]iceberg.NestedField, len(cfg.Fields))
	for i, f := range cfg.Fields {
		id := f.ID
		if id == 0 {
			id = i + 1
		}
		var typ iceberg.Type
		switch strings.ToLower(f.Type) {
		case "int", "int32", "int64", "integer":
			typ = iceberg.PrimitiveTypes.Int64
		case "float", "float64", "double", "number":
			typ = iceberg.PrimitiveTypes.Float64
		case "string", "text":
			typ = iceberg.PrimitiveTypes.String
		case "bool", "boolean":
			typ = iceberg.PrimitiveTypes.Bool
		case "timestamp", "timestamptz", "datetime":
			typ = iceberg.PrimitiveTypes.TimestampTz
		case "binary", "bytes":
			typ = iceberg.PrimitiveTypes.Binary
		case "map", "map[string]float64":
			typ = &iceberg.MapType{
				KeyID: id*100 + 1, KeyType: iceberg.PrimitiveTypes.String,
				ValueID: id*100 + 2, ValueType: iceberg.PrimitiveTypes.Float64,
			}
		case "map[string]string":
			typ = &iceberg.MapType{
				KeyID: id*100 + 1, KeyType: iceberg.PrimitiveTypes.String,
				ValueID: id*100 + 2, ValueType: iceberg.PrimitiveTypes.String,
			}
		default:
			typ = iceberg.PrimitiveTypes.String
		}

		fields[i] = iceberg.NestedField{
			ID:       id,
			Name:     f.Name,
			Type:     typ,
			Required: f.Required,
		}
	}

	return iceberg.NewSchema(0, fields...), nil
}


const (
	Namespace    = "hindsight"
	SpotTicker   = "spot_ticker"
	SpotTrade    = "spot_trade"
	SpotLevel3   = "spot_level3"
	Measurements = "measurements"
	Runs         = "runs"
	Excursions   = "excursions"
)

/*
MeasurementSchema defines the canonical tabular representation of *data.Measurement[float64].
Top-level columns epoch, tick, and symbol allow Iceberg partition pruning and file-level
min/max metric pruning before Parquet pages are fetched.
*/
func MeasurementSchema() *iceberg.Schema {
	return iceberg.NewSchema(0,
		iceberg.NestedField{ID: 1, Name: "epoch", Type: iceberg.PrimitiveTypes.Int64, Required: true},
		iceberg.NestedField{ID: 2, Name: "tick", Type: iceberg.PrimitiveTypes.Int64, Required: true},
		iceberg.NestedField{ID: 3, Name: "source", Type: iceberg.PrimitiveTypes.String, Required: true},
		iceberg.NestedField{ID: 4, Name: "symbol", Type: iceberg.PrimitiveTypes.String, Required: true},
		iceberg.NestedField{ID: 5, Name: "venue_at", Type: iceberg.PrimitiveTypes.TimestampTz, Required: true},
		iceberg.NestedField{ID: 6, Name: "maturity", Type: iceberg.PrimitiveTypes.Float64, Required: true},
		iceberg.NestedField{ID: 7, Name: "snr", Type: iceberg.PrimitiveTypes.Float64, Required: false},
		iceberg.NestedField{ID: 8, Name: "snr_defined", Type: iceberg.PrimitiveTypes.Bool, Required: true},
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
MeasurementPartitioning isolates runs physically by identity partitioning on epoch.
*/
func MeasurementPartitioning() iceberg.PartitionSpec {
	return iceberg.NewPartitionSpec(
		iceberg.PartitionField{
			SourceIDs: []int{1},
			FieldID:   1000,
			Name:      "epoch",
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

/*
ExcursionsSchema defines the tabular representation of verified market excursions.
Top-level epoch partition allows run isolation and quick pruning during training queries.
*/
func ExcursionsSchema() *iceberg.Schema {
	return iceberg.NewSchema(0,
		iceberg.NestedField{ID: 1, Name: "epoch", Type: iceberg.PrimitiveTypes.Int64, Required: true},
		iceberg.NestedField{ID: 2, Name: "id", Type: iceberg.PrimitiveTypes.String, Required: true},
		iceberg.NestedField{ID: 3, Name: "symbol", Type: iceberg.PrimitiveTypes.String, Required: true},
		iceberg.NestedField{ID: 4, Name: "direction", Type: iceberg.PrimitiveTypes.String, Required: true},
		iceberg.NestedField{ID: 5, Name: "clears_friction", Type: iceberg.PrimitiveTypes.Bool, Required: true},
		iceberg.NestedField{ID: 6, Name: "precursor_start_tick", Type: iceberg.PrimitiveTypes.Int64, Required: true},
		iceberg.NestedField{ID: 7, Name: "anchor_tick", Type: iceberg.PrimitiveTypes.Int64, Required: true},
		iceberg.NestedField{ID: 8, Name: "extremum_tick", Type: iceberg.PrimitiveTypes.Int64, Required: true},
		iceberg.NestedField{ID: 9, Name: "exit_tick", Type: iceberg.PrimitiveTypes.Int64, Required: true},
		iceberg.NestedField{ID: 10, Name: "post_end_tick", Type: iceberg.PrimitiveTypes.Int64, Required: true},
		iceberg.NestedField{ID: 11, Name: "entry_price", Type: iceberg.PrimitiveTypes.Float64, Required: true},
		iceberg.NestedField{ID: 12, Name: "extremum_price", Type: iceberg.PrimitiveTypes.Float64, Required: true},
		iceberg.NestedField{ID: 13, Name: "exit_price", Type: iceberg.PrimitiveTypes.Float64, Required: true},
		iceberg.NestedField{ID: 14, Name: "position_size", Type: iceberg.PrimitiveTypes.Float64, Required: true},
		iceberg.NestedField{ID: 15, Name: "fee", Type: iceberg.PrimitiveTypes.Float64, Required: true},
		iceberg.NestedField{ID: 16, Name: "profit", Type: iceberg.PrimitiveTypes.Float64, Required: true},
		iceberg.NestedField{ID: 17, Name: "profit_fraction", Type: iceberg.PrimitiveTypes.Float64, Required: true},
		iceberg.NestedField{ID: 18, Name: "gross_excursion", Type: iceberg.PrimitiveTypes.Float64, Required: true},
		iceberg.NestedField{ID: 19, Name: "observation_count", Type: iceberg.PrimitiveTypes.Int64, Required: true},
		iceberg.NestedField{ID: 20, Name: "status", Type: iceberg.PrimitiveTypes.String, Required: true},
	)
}

/*
ExcursionsPartitioning isolates runs physically by identity partitioning on epoch.
*/
func ExcursionsPartitioning() iceberg.PartitionSpec {
	return iceberg.NewPartitionSpec(
		iceberg.PartitionField{
			SourceIDs: []int{1},
			FieldID:   1000,
			Name:      "epoch",
			Transform: iceberg.IdentityTransform{},
		},
	)
}
