package types

import wire "github.com/theapemachine/symm/telemetry/generated/telemetry"

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
		Stoploss:        StoplossWire(holding.Stoploss),
	}
}

func StoplossWire(stoploss *Stoploss) *wire.StoplossT {
	if stoploss == nil {
		return nil
	}

	return &wire.StoplossT{
		Status:        string(stoploss.Status),
		Symbol:        stoploss.Symbol,
		Floor:         decimalString(stoploss.Floor),
		Mark:          decimalString(stoploss.Mark),
		Peak:          decimalString(stoploss.Peak),
		ProfitLine:    decimalString(stoploss.ProfitLine),
		ArmAt:         decimalString(stoploss.ArmAt),
		LockFloor:     decimalString(stoploss.LockFloor),
		Locked:        stoploss.Locked,
		TriggerReason: stoploss.TriggerReason,
		TriggerMark:   decimalString(stoploss.TriggerMark),
		SurgeArmed:    stoploss.SurgeArmed,
		LastMove:      decimalString(stoploss.LastMove),
		SurgeMove:     decimalString(stoploss.SurgeMove),
		MomentumFloor: decimalString(stoploss.MomentumFloor),
		Plan:          riskWire(stoploss.Plan),
	}
}

func entryCostWire(cost *EntryCost) *wire.EntryCostT {
	if cost == nil {
		return nil
	}

	return &wire.EntryCostT{
		EntryPrice:         decimalString(cost.EntryPrice),
		BestAsk:            decimalString(cost.BestAsk),
		BestBid:            decimalString(cost.BestBid),
		Midpoint:           decimalString(cost.Midpoint),
		GrossNotional:      decimalString(cost.GrossNotional),
		EntryFee:           decimalString(cost.EntryFee),
		ExitFeeAtBreakEven: decimalString(cost.ExitFeeAtBreakEven),
		RoundTripFees:      decimalString(cost.RoundTripFees),
		Spread:             decimalString(cost.Spread),
		Impact:             decimalString(cost.Impact),
		BreakEven:          decimalString(cost.BreakEven),
	}
}

func riskWire(risk *RiskPlan) *wire.RiskPlanT {
	if risk == nil {
		return nil
	}

	return &wire.RiskPlanT{
		Present:        risk.Present,
		EntryNoiseBand: decimalString(risk.EntryNoiseBand),
		NoiseBand:      decimalString(risk.NoiseBand),
		RiskDistance:   decimalString(risk.RiskDistance),
		TrailDistance:  decimalString(risk.TrailDistance),
		ArmBuffer:      decimalString(risk.ArmBuffer),
		LockBuffer:     decimalString(risk.LockBuffer),
		MinEdge:        decimalString(risk.MinEdge),
		MaxLoss:        decimalString(risk.MaxLoss),
		ExitFeeRate:    decimalString(risk.ExitFeeRate),
		EntryFeeRate:   decimalString(risk.EntryFeeRate),
	}
}
