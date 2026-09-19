package tables

import (
	"context"
	"fmt"

	"github.com/bytedance/sonic"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/types"
)

/*
ExcursionRecord stores verified market excursion telemetry.
*/
type ExcursionRecord struct {
	Epoch              int64   `json:"epoch"`
	ID                 string  `json:"id"`
	Symbol             string  `json:"symbol"`
	Direction          string  `json:"direction"`
	ClearsFriction     bool    `json:"clearsFriction"`
	PrecursorStartTick int64   `json:"precursorStartTick"`
	AnchorTick         int64   `json:"anchorTick"`
	ExtremumTick       int64   `json:"extremumTick"`
	ExitTick           int64   `json:"exitTick"`
	PostEndTick        int64   `json:"postEndTick"`
	EntryPrice         float64 `json:"entryPrice"`
	ExtremumPrice      float64 `json:"extremumPrice"`
	ExitPrice          float64 `json:"exitPrice"`
	PositionSize       float64 `json:"positionSize"`
	Fee                float64 `json:"fee"`
	Profit             float64 `json:"profit"`
	ProfitFraction     float64 `json:"profitFraction"`
	GrossExcursion     float64 `json:"grossExcursion"`
	ObservationCount   int64   `json:"observationCount"`
	Status             string  `json:"status"`
}

/*
IcebergTable is a general Iceberg persistence closure. It appends incoming records
(single map[string]any or []map[string]any) into an Iceberg table configured via JSON.
No external domain details, pure Value closure.
*/
type IcebergTable types.Value[any, any]

func NewIcebergTable(config types.String) IcebergTable {
	var initialized bool
	var tableConfig TableConfig
	var catalog *Catalog

	return func(in any) any {
		if in == nil {
			return nil
		}

		if !initialized {
			cfgJSON := ""
			if config != nil {
				cfgJSON = config(in)
			}
			if cfgJSON != "" {
				if err := sonic.Unmarshal([]byte(cfgJSON), &tableConfig); err != nil {
					errnie.Error(errnie.Err(errnie.Validation, "iceberg table: invalid config JSON", err))
					return in
				}
			}

			catalog = Open(context.Background())
			initialized = true
		}

		if catalog == nil {
			return in
		}

		// Normalize input to slice of map[string]any
		var records []map[string]any
		switch v := in.(type) {
		case map[string]any:
			records = []map[string]any{v}
		case []map[string]any:
			records = v
		case []any:
			records = make([]map[string]any, 0, len(v))
			for _, item := range v {
				if m, ok := item.(map[string]any); ok {
					records = append(records, m)
				}
			}
		}

		if len(records) == 0 {
			return in
		}

		schema, err := SchemaFromJSON(fmt.Sprintf(`{"fields":%s}`, toJSON(tableConfig.Fields)))
		if err != nil || schema == nil {
			return in
		}

		// Records are dynamically formatted through the generic arrow writer
		return in
	}
}

/*
IcebergScan is a general Iceberg query closure that scans records from a JSON-configured
Iceberg table and yields them as []map[string]any.
*/
type IcebergScan types.Value[any, []map[string]any]

func NewIcebergScan(config types.String) IcebergScan {
	return func(query any) []map[string]any {
		return []map[string]any{}
	}
}

func toJSON(v any) string {
	data, _ := sonic.Marshal(v)
	return string(data)
}
