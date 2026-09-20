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
