package tables

import (
	"github.com/apache/iceberg-go"
)

const (
	Namespace     = "hindsight"
	SpotLevel3    = "spot_level3"
	SpotTicker    = "spot_ticker"
	SpotTrade     = "spot_trade"
	FuturesTicker = "futures_ticker"
	FuturesTrade  = "futures_trade"
	Executions    = "executions"
	Measurements  = "measurements"
	Models        = "models"
	Grids         = "grids"
	Positions     = "positions"
	Decisions     = "decisions"
	Outcomes      = "outcomes"

	DecimalPrecision = 38
	DecimalScale     = 18
)

func moneyType() iceberg.Type {
	return iceberg.DecimalTypeOf(DecimalPrecision, DecimalScale)
}

func SpotLevel3Schema() *iceberg.Schema {
	return iceberg.NewSchema(0,
		iceberg.NestedField{ID: 1, Name: "epoch", Type: iceberg.PrimitiveTypes.Int64, Required: true},
		iceberg.NestedField{ID: 2, Name: "tick", Type: iceberg.PrimitiveTypes.Int64, Required: true},
		iceberg.NestedField{ID: 3, Name: "symbol", Type: iceberg.PrimitiveTypes.String, Required: true},
		iceberg.NestedField{ID: 4, Name: "venue_at", Type: iceberg.PrimitiveTypes.TimestampTz, Required: true},
		iceberg.NestedField{ID: 5, Name: "received_at", Type: iceberg.PrimitiveTypes.TimestampTz, Required: true},
		iceberg.NestedField{ID: 6, Name: "side", Type: iceberg.PrimitiveTypes.String, Required: true},
		iceberg.NestedField{ID: 7, Name: "event", Type: iceberg.PrimitiveTypes.String, Required: true},
		iceberg.NestedField{ID: 8, Name: "order_id", Type: iceberg.PrimitiveTypes.String, Required: true},
		iceberg.NestedField{ID: 9, Name: "limit_price", Type: iceberg.PrimitiveTypes.Float64, Required: true},
		iceberg.NestedField{ID: 10, Name: "order_qty", Type: iceberg.PrimitiveTypes.Float64, Required: true},
		iceberg.NestedField{ID: 11, Name: "checksum", Type: iceberg.PrimitiveTypes.Int64, Required: true},
	)
}

func SpotTickerSchema() *iceberg.Schema {
	return iceberg.NewSchema(0,
		iceberg.NestedField{ID: 1, Name: "epoch", Type: iceberg.PrimitiveTypes.Int64, Required: true},
		iceberg.NestedField{ID: 2, Name: "tick", Type: iceberg.PrimitiveTypes.Int64, Required: true},
		iceberg.NestedField{ID: 3, Name: "symbol", Type: iceberg.PrimitiveTypes.String, Required: true},
		iceberg.NestedField{ID: 4, Name: "venue_at", Type: iceberg.PrimitiveTypes.TimestampTz, Required: true},
		iceberg.NestedField{ID: 5, Name: "received_at", Type: iceberg.PrimitiveTypes.TimestampTz, Required: true},
		iceberg.NestedField{ID: 6, Name: "bid", Type: iceberg.PrimitiveTypes.Float64, Required: true},
		iceberg.NestedField{ID: 7, Name: "bid_qty", Type: iceberg.PrimitiveTypes.Float64, Required: true},
		iceberg.NestedField{ID: 8, Name: "ask", Type: iceberg.PrimitiveTypes.Float64, Required: true},
		iceberg.NestedField{ID: 9, Name: "ask_qty", Type: iceberg.PrimitiveTypes.Float64, Required: true},
		iceberg.NestedField{ID: 10, Name: "last", Type: iceberg.PrimitiveTypes.Float64, Required: true},
		iceberg.NestedField{ID: 11, Name: "volume", Type: iceberg.PrimitiveTypes.Float64, Required: true},
		iceberg.NestedField{ID: 12, Name: "vwap", Type: iceberg.PrimitiveTypes.Float64, Required: true},
		iceberg.NestedField{ID: 13, Name: "low", Type: iceberg.PrimitiveTypes.Float64, Required: true},
		iceberg.NestedField{ID: 14, Name: "high", Type: iceberg.PrimitiveTypes.Float64, Required: true},
		iceberg.NestedField{ID: 15, Name: "change", Type: iceberg.PrimitiveTypes.Float64, Required: true},
		iceberg.NestedField{ID: 16, Name: "change_pct", Type: iceberg.PrimitiveTypes.Float64, Required: true},
	)
}

func SpotTradeSchema() *iceberg.Schema {
	return iceberg.NewSchema(0,
		iceberg.NestedField{ID: 1, Name: "epoch", Type: iceberg.PrimitiveTypes.Int64, Required: true},
		iceberg.NestedField{ID: 2, Name: "tick", Type: iceberg.PrimitiveTypes.Int64, Required: true},
		iceberg.NestedField{ID: 3, Name: "symbol", Type: iceberg.PrimitiveTypes.String, Required: true},
		iceberg.NestedField{ID: 4, Name: "venue_at", Type: iceberg.PrimitiveTypes.TimestampTz, Required: true},
		iceberg.NestedField{ID: 5, Name: "received_at", Type: iceberg.PrimitiveTypes.TimestampTz, Required: true},
		iceberg.NestedField{ID: 6, Name: "price", Type: iceberg.PrimitiveTypes.Float64, Required: true},
		iceberg.NestedField{ID: 7, Name: "qty", Type: iceberg.PrimitiveTypes.Float64, Required: true},
		iceberg.NestedField{ID: 8, Name: "side", Type: iceberg.PrimitiveTypes.String, Required: true},
		iceberg.NestedField{ID: 9, Name: "ord_type", Type: iceberg.PrimitiveTypes.String, Required: true},
		iceberg.NestedField{ID: 10, Name: "trade_id", Type: iceberg.PrimitiveTypes.Int64, Required: true},
	)
}

func FuturesTickerSchema() *iceberg.Schema {
	return iceberg.NewSchema(0,
		iceberg.NestedField{ID: 1, Name: "epoch", Type: iceberg.PrimitiveTypes.Int64, Required: true},
		iceberg.NestedField{ID: 2, Name: "tick", Type: iceberg.PrimitiveTypes.Int64, Required: true},
		iceberg.NestedField{ID: 3, Name: "symbol", Type: iceberg.PrimitiveTypes.String, Required: true},
		iceberg.NestedField{ID: 4, Name: "venue_at", Type: iceberg.PrimitiveTypes.TimestampTz, Required: true},
		iceberg.NestedField{ID: 5, Name: "received_at", Type: iceberg.PrimitiveTypes.TimestampTz, Required: true},
		iceberg.NestedField{ID: 6, Name: "bid", Type: iceberg.PrimitiveTypes.Float64, Required: true},
		iceberg.NestedField{ID: 7, Name: "bid_qty", Type: iceberg.PrimitiveTypes.Float64, Required: true},
		iceberg.NestedField{ID: 8, Name: "ask", Type: iceberg.PrimitiveTypes.Float64, Required: true},
		iceberg.NestedField{ID: 9, Name: "ask_qty", Type: iceberg.PrimitiveTypes.Float64, Required: true},
		iceberg.NestedField{ID: 10, Name: "last", Type: iceberg.PrimitiveTypes.Float64, Required: true},
		iceberg.NestedField{ID: 11, Name: "volume", Type: iceberg.PrimitiveTypes.Float64, Required: true},
		iceberg.NestedField{ID: 12, Name: "vwap", Type: iceberg.PrimitiveTypes.Float64, Required: true},
		iceberg.NestedField{ID: 13, Name: "low", Type: iceberg.PrimitiveTypes.Float64, Required: true},
		iceberg.NestedField{ID: 14, Name: "high", Type: iceberg.PrimitiveTypes.Float64, Required: true},
		iceberg.NestedField{ID: 15, Name: "change", Type: iceberg.PrimitiveTypes.Float64, Required: true},
		iceberg.NestedField{ID: 16, Name: "change_pct", Type: iceberg.PrimitiveTypes.Float64, Required: true},
		iceberg.NestedField{ID: 17, Name: "mark_price", Type: iceberg.PrimitiveTypes.Float64, Required: true},
		iceberg.NestedField{ID: 18, Name: "index_price", Type: iceberg.PrimitiveTypes.Float64, Required: true},
		iceberg.NestedField{ID: 19, Name: "open_interest", Type: iceberg.PrimitiveTypes.Float64, Required: true},
	)
}

func FuturesTradeSchema() *iceberg.Schema {
	return iceberg.NewSchema(0,
		iceberg.NestedField{ID: 1, Name: "epoch", Type: iceberg.PrimitiveTypes.Int64, Required: true},
		iceberg.NestedField{ID: 2, Name: "tick", Type: iceberg.PrimitiveTypes.Int64, Required: true},
		iceberg.NestedField{ID: 3, Name: "symbol", Type: iceberg.PrimitiveTypes.String, Required: true},
		iceberg.NestedField{ID: 4, Name: "venue_at", Type: iceberg.PrimitiveTypes.TimestampTz, Required: true},
		iceberg.NestedField{ID: 5, Name: "received_at", Type: iceberg.PrimitiveTypes.TimestampTz, Required: true},
		iceberg.NestedField{ID: 6, Name: "price", Type: iceberg.PrimitiveTypes.Float64, Required: true},
		iceberg.NestedField{ID: 7, Name: "qty", Type: iceberg.PrimitiveTypes.Float64, Required: true},
		iceberg.NestedField{ID: 8, Name: "side", Type: iceberg.PrimitiveTypes.String, Required: true},
		iceberg.NestedField{ID: 9, Name: "ord_type", Type: iceberg.PrimitiveTypes.String, Required: true},
		iceberg.NestedField{ID: 10, Name: "trade_id", Type: iceberg.PrimitiveTypes.Int64, Required: true},
	)
}

func ExecutionsSchema() *iceberg.Schema {
	return iceberg.NewSchema(0,
		iceberg.NestedField{ID: 1, Name: "epoch", Type: iceberg.PrimitiveTypes.Int64, Required: true},
		iceberg.NestedField{ID: 2, Name: "tick", Type: iceberg.PrimitiveTypes.Int64, Required: true},
		iceberg.NestedField{ID: 3, Name: "symbol", Type: iceberg.PrimitiveTypes.String, Required: true},
		iceberg.NestedField{ID: 4, Name: "venue_at", Type: iceberg.PrimitiveTypes.TimestampTz, Required: true},
		iceberg.NestedField{ID: 5, Name: "received_at", Type: iceberg.PrimitiveTypes.TimestampTz, Required: true},
		iceberg.NestedField{ID: 6, Name: "order_id", Type: iceberg.PrimitiveTypes.String, Required: true},
		iceberg.NestedField{ID: 7, Name: "order_userref", Type: iceberg.PrimitiveTypes.Int64, Required: true},
		iceberg.NestedField{ID: 8, Name: "exec_id", Type: iceberg.PrimitiveTypes.String, Required: true},
		iceberg.NestedField{ID: 9, Name: "exec_type", Type: iceberg.PrimitiveTypes.String, Required: true},
		iceberg.NestedField{ID: 10, Name: "trade_id", Type: iceberg.PrimitiveTypes.Int64, Required: true},
		iceberg.NestedField{ID: 11, Name: "side", Type: iceberg.PrimitiveTypes.String, Required: true},
		iceberg.NestedField{ID: 12, Name: "last_qty", Type: moneyType(), Required: true},
		iceberg.NestedField{ID: 13, Name: "last_price", Type: moneyType(), Required: true},
		iceberg.NestedField{ID: 14, Name: "liquidity_ind", Type: iceberg.PrimitiveTypes.String, Required: true},
		iceberg.NestedField{ID: 15, Name: "cost", Type: moneyType(), Required: true},
		iceberg.NestedField{ID: 16, Name: "order_type", Type: iceberg.PrimitiveTypes.String, Required: true},
		iceberg.NestedField{ID: 17, Name: "order_status", Type: iceberg.PrimitiveTypes.String, Required: true},
		iceberg.NestedField{ID: 18, Name: "cum_qty", Type: moneyType(), Required: true},
		iceberg.NestedField{ID: 19, Name: "cum_cost", Type: moneyType(), Required: true},
		iceberg.NestedField{ID: 20, Name: "avg_price", Type: moneyType(), Required: true},
		iceberg.NestedField{ID: 21, Name: "fee_usd_equiv", Type: moneyType(), Required: true},
		iceberg.NestedField{ID: 22, Name: "fees", Type: iceberg.PrimitiveTypes.String, Required: true},
	)
}

func MeasurementsSchema() *iceberg.Schema {
	return iceberg.NewSchema(0,
		iceberg.NestedField{ID: 1, Name: "epoch", Type: iceberg.PrimitiveTypes.Int64, Required: true},
		iceberg.NestedField{ID: 2, Name: "tick", Type: iceberg.PrimitiveTypes.Int64, Required: true},
		iceberg.NestedField{ID: 3, Name: "source", Type: iceberg.PrimitiveTypes.String, Required: true},
		iceberg.NestedField{ID: 4, Name: "symbol", Type: iceberg.PrimitiveTypes.String, Required: true},
		iceberg.NestedField{ID: 5, Name: "venue_at", Type: iceberg.PrimitiveTypes.TimestampTz},
		iceberg.NestedField{ID: 6, Name: "observed_at", Type: iceberg.PrimitiveTypes.TimestampTz, Required: true},
		iceberg.NestedField{ID: 7, Name: "maturity", Type: iceberg.PrimitiveTypes.Float64, Required: true},
		iceberg.NestedField{ID: 8, Name: "snr", Type: iceberg.PrimitiveTypes.Float64},
		iceberg.NestedField{ID: 9, Name: "snr_defined", Type: iceberg.PrimitiveTypes.Bool, Required: true},
		iceberg.NestedField{ID: 10, Name: "metrics", Required: false,
			Type: &iceberg.MapType{
				KeyID: 1001, KeyType: iceberg.PrimitiveTypes.String,
				ValueID: 1002, ValueType: iceberg.PrimitiveTypes.Float64, ValueRequired: false,
			},
		},
		iceberg.NestedField{ID: 11, Name: "metadata", Required: false,
			Type: &iceberg.MapType{
				KeyID: 1101, KeyType: iceberg.PrimitiveTypes.String,
				ValueID: 1102, ValueType: iceberg.PrimitiveTypes.Float64, ValueRequired: false,
			},
		},
		iceberg.NestedField{ID: 12, Name: "payload", Type: iceberg.PrimitiveTypes.Binary},
	)
}

func ModelsSchema() *iceberg.Schema {
	return iceberg.NewSchema(0,
		iceberg.NestedField{ID: 1, Name: "epoch", Type: iceberg.PrimitiveTypes.Int64, Required: true},
		iceberg.NestedField{ID: 2, Name: "tick", Type: iceberg.PrimitiveTypes.Int64, Required: true},
		iceberg.NestedField{ID: 3, Name: "agent_id", Type: iceberg.PrimitiveTypes.Int32, Required: true},
		iceberg.NestedField{ID: 4, Name: "step_count", Type: iceberg.PrimitiveTypes.Int64, Required: true},
		iceberg.NestedField{ID: 5, Name: "nodes_count", Type: iceberg.PrimitiveTypes.Int64, Required: true},
		iceberg.NestedField{ID: 6, Name: "payload", Type: iceberg.PrimitiveTypes.Binary, Required: true},
	)
}

func GridsSchema() *iceberg.Schema {
	return iceberg.NewSchema(0,
		iceberg.NestedField{ID: 1, Name: "epoch", Type: iceberg.PrimitiveTypes.Int64, Required: true},
		iceberg.NestedField{ID: 2, Name: "tick", Type: iceberg.PrimitiveTypes.Int64, Required: true},
		iceberg.NestedField{ID: 3, Name: "agent_id", Type: iceberg.PrimitiveTypes.Int32, Required: true},
		iceberg.NestedField{ID: 4, Name: "context_label", Type: iceberg.PrimitiveTypes.String, Required: true},
		iceberg.NestedField{ID: 5, Name: "payload", Type: iceberg.PrimitiveTypes.Binary, Required: true},
	)
}

func PositionsSchema() *iceberg.Schema {
	return iceberg.NewSchema(0,
		iceberg.NestedField{ID: 1, Name: "epoch", Type: iceberg.PrimitiveTypes.Int64, Required: true},
		iceberg.NestedField{ID: 2, Name: "tick", Type: iceberg.PrimitiveTypes.Int64, Required: true},
		iceberg.NestedField{ID: 3, Name: "symbol", Type: iceberg.PrimitiveTypes.String, Required: true},
		iceberg.NestedField{ID: 4, Name: "status", Type: iceberg.PrimitiveTypes.String, Required: true},
		iceberg.NestedField{ID: 5, Name: "qty", Type: moneyType(), Required: true},
		iceberg.NestedField{ID: 6, Name: "basis", Type: moneyType()},
		iceberg.NestedField{ID: 7, Name: "entry_price", Type: moneyType()},
		iceberg.NestedField{ID: 8, Name: "entry_fee", Type: moneyType()},
		iceberg.NestedField{ID: 9, Name: "exit_price", Type: moneyType()},
		iceberg.NestedField{ID: 10, Name: "exit_fee", Type: moneyType()},
		iceberg.NestedField{ID: 11, Name: "mark", Type: moneyType()},
		iceberg.NestedField{ID: 12, Name: "pnl", Type: moneyType()},
		iceberg.NestedField{ID: 13, Name: "realized_pnl", Type: moneyType()},
		iceberg.NestedField{ID: 14, Name: "entry_at", Type: iceberg.PrimitiveTypes.TimestampTz},
		iceberg.NestedField{ID: 15, Name: "exit_at", Type: iceberg.PrimitiveTypes.TimestampTz},
	)
}

func DecisionsSchema() *iceberg.Schema { return OutcomesSchema() }

func OutcomesSchema() *iceberg.Schema {
	return iceberg.NewSchema(0,
		iceberg.NestedField{ID: 1, Name: "epoch", Type: iceberg.PrimitiveTypes.Int64, Required: true},
		iceberg.NestedField{ID: 2, Name: "tick", Type: iceberg.PrimitiveTypes.Int64, Required: true},
		iceberg.NestedField{ID: 3, Name: "decision_id", Type: iceberg.PrimitiveTypes.Int64, Required: true},
		iceberg.NestedField{ID: 4, Name: "symbol", Type: iceberg.PrimitiveTypes.String, Required: true},
		iceberg.NestedField{ID: 5, Name: "at", Type: iceberg.PrimitiveTypes.TimestampTz, Required: true},
		iceberg.NestedField{ID: 6, Name: "action_kind", Type: iceberg.PrimitiveTypes.String, Required: true},
		iceberg.NestedField{ID: 7, Name: "action_power", Type: iceberg.PrimitiveTypes.Int32, Required: true},
		iceberg.NestedField{ID: 8, Name: "action_reduce", Type: iceberg.PrimitiveTypes.Bool, Required: true},
		iceberg.NestedField{ID: 9, Name: "authority", Type: iceberg.PrimitiveTypes.Float64, Required: true},
		iceberg.NestedField{ID: 10, Name: "outcome", Type: iceberg.PrimitiveTypes.Float64},
	)
}
