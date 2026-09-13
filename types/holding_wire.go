package types

import (
	"time"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	wire "github.com/theapemachine/symm/telemetry/generated/telemetry"
)

func decimalString(value *decimal.Decimal) string {
	if value == nil {
		return ""
	}

	return value.String()
}

func timeNs(at time.Time) int64 {
	return at.UnixNano()
}

func timePointerNano(at *time.Time) int64 {
	if at == nil {
		return 0
	}

	return timeNs(*at)
}

func HoldingWire(holding *Holding) *wire.HoldingT {
	if holding == nil {
		return nil
	}

	return &wire.HoldingT{
		Status:          string(holding.Status),
		Symbol:          holding.Symbol,
		Asset:           holding.Asset,
		Qty:             decimalString(holding.Qty),
		SellableQty:     decimalString(holding.SellableQty),
		EntryAt:         timePointerNano(holding.EntryAt),
		ExitAt:          timePointerNano(holding.ExitAt),
		EntryPrice:      decimalString(holding.EntryPrice),
		EntryFee:        decimalString(holding.EntryFee),
		ExitPrice:       decimalString(holding.ExitPrice),
		ExitFee:         decimalString(holding.ExitFee),
		Pnl:             decimalString(holding.PnL),
		ProfitThreshold: decimalString(holding.ProfitThreshold),
		ReturnPct:       holding.ReturnPct,
		Mark:            decimalString(holding.Mark),
		IsOpportunity:   holding.IsOpportunity,
		ReservationId:   holding.ReservationID,
	}
}
