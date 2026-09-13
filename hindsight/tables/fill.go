package tables

import (
	"github.com/apache/arrow-go/v18/arrow/array"
)

func fillSpotLevel3(builder *array.RecordBuilder, rows []SpotLevel3Row) {
	epoch := builder.Field(0).(*array.Int64Builder)
	tick := builder.Field(1).(*array.Int64Builder)
	symbol := builder.Field(2).(*array.StringBuilder)
	venueAt := builder.Field(3).(*array.TimestampBuilder)
	receivedAt := builder.Field(4).(*array.TimestampBuilder)
	side := builder.Field(5).(*array.StringBuilder)
	event := builder.Field(6).(*array.StringBuilder)
	orderID := builder.Field(7).(*array.StringBuilder)
	limitPrice := builder.Field(8).(*array.Float64Builder)
	orderQty := builder.Field(9).(*array.Float64Builder)
	checksum := builder.Field(10).(*array.Int64Builder)

	for _, row := range rows {
		epoch.Append(row.Epoch)
		tick.Append(row.Tick)
		text(symbol, row.Symbol)
		timestamp(venueAt, row.VenueAt)
		timestamp(receivedAt, row.ReceivedAt)
		text(side, row.Side)
		text(event, row.Event)
		text(orderID, row.OrderID)
		limitPrice.Append(row.LimitPrice)
		orderQty.Append(row.OrderQty)
		checksum.Append(row.Checksum)
	}
}

func fillSpotTicker(builder *array.RecordBuilder, rows []SpotTickerRow) {
	epoch := builder.Field(0).(*array.Int64Builder)
	tick := builder.Field(1).(*array.Int64Builder)
	symbol := builder.Field(2).(*array.StringBuilder)
	venueAt := builder.Field(3).(*array.TimestampBuilder)
	receivedAt := builder.Field(4).(*array.TimestampBuilder)
	bid := builder.Field(5).(*array.Float64Builder)
	bidQty := builder.Field(6).(*array.Float64Builder)
	ask := builder.Field(7).(*array.Float64Builder)
	askQty := builder.Field(8).(*array.Float64Builder)
	last := builder.Field(9).(*array.Float64Builder)
	volume := builder.Field(10).(*array.Float64Builder)
	vwap := builder.Field(11).(*array.Float64Builder)
	low := builder.Field(12).(*array.Float64Builder)
	high := builder.Field(13).(*array.Float64Builder)
	change := builder.Field(14).(*array.Float64Builder)
	changePct := builder.Field(15).(*array.Float64Builder)

	for _, row := range rows {
		epoch.Append(row.Epoch)
		tick.Append(row.Tick)
		text(symbol, row.Symbol)
		timestamp(venueAt, row.VenueAt)
		timestamp(receivedAt, row.ReceivedAt)
		bid.Append(row.Bid)
		bidQty.Append(row.BidQty)
		ask.Append(row.Ask)
		askQty.Append(row.AskQty)
		last.Append(row.Last)
		volume.Append(row.Volume)
		vwap.Append(row.VWAP)
		low.Append(row.Low)
		high.Append(row.High)
		change.Append(row.Change)
		changePct.Append(row.ChangePct)
	}
}

func fillSpotTrade(builder *array.RecordBuilder, rows []SpotTradeRow) {
	epoch := builder.Field(0).(*array.Int64Builder)
	tick := builder.Field(1).(*array.Int64Builder)
	symbol := builder.Field(2).(*array.StringBuilder)
	venueAt := builder.Field(3).(*array.TimestampBuilder)
	receivedAt := builder.Field(4).(*array.TimestampBuilder)
	price := builder.Field(5).(*array.Float64Builder)
	qty := builder.Field(6).(*array.Float64Builder)
	side := builder.Field(7).(*array.StringBuilder)
	ordType := builder.Field(8).(*array.StringBuilder)
	tradeID := builder.Field(9).(*array.Int64Builder)

	for _, row := range rows {
		epoch.Append(row.Epoch)
		tick.Append(row.Tick)
		text(symbol, row.Symbol)
		timestamp(venueAt, row.VenueAt)
		timestamp(receivedAt, row.ReceivedAt)
		price.Append(row.Price)
		qty.Append(row.Qty)
		text(side, row.Side)
		text(ordType, row.OrdType)
		tradeID.Append(row.TradeID)
	}
}

func fillFuturesTicker(builder *array.RecordBuilder, rows []FuturesTickerRow) {
	epoch := builder.Field(0).(*array.Int64Builder)
	tick := builder.Field(1).(*array.Int64Builder)
	symbol := builder.Field(2).(*array.StringBuilder)
	venueAt := builder.Field(3).(*array.TimestampBuilder)
	receivedAt := builder.Field(4).(*array.TimestampBuilder)
	bid := builder.Field(5).(*array.Float64Builder)
	bidQty := builder.Field(6).(*array.Float64Builder)
	ask := builder.Field(7).(*array.Float64Builder)
	askQty := builder.Field(8).(*array.Float64Builder)
	last := builder.Field(9).(*array.Float64Builder)
	volume := builder.Field(10).(*array.Float64Builder)
	vwap := builder.Field(11).(*array.Float64Builder)
	low := builder.Field(12).(*array.Float64Builder)
	high := builder.Field(13).(*array.Float64Builder)
	change := builder.Field(14).(*array.Float64Builder)
	changePct := builder.Field(15).(*array.Float64Builder)
	markPrice := builder.Field(16).(*array.Float64Builder)
	indexPrice := builder.Field(17).(*array.Float64Builder)
	openInterest := builder.Field(18).(*array.Float64Builder)

	for _, row := range rows {
		epoch.Append(row.Epoch)
		tick.Append(row.Tick)
		text(symbol, row.Symbol)
		timestamp(venueAt, row.VenueAt)
		timestamp(receivedAt, row.ReceivedAt)
		bid.Append(row.Bid)
		bidQty.Append(row.BidQty)
		ask.Append(row.Ask)
		askQty.Append(row.AskQty)
		last.Append(row.Last)
		volume.Append(row.Volume)
		vwap.Append(row.VWAP)
		low.Append(row.Low)
		high.Append(row.High)
		change.Append(row.Change)
		changePct.Append(row.ChangePct)
		markPrice.Append(row.MarkPrice)
		indexPrice.Append(row.IndexPrice)
		openInterest.Append(row.OpenInterest)
	}
}

func fillFuturesTrade(builder *array.RecordBuilder, rows []FuturesTradeRow) {
	epoch := builder.Field(0).(*array.Int64Builder)
	tick := builder.Field(1).(*array.Int64Builder)
	symbol := builder.Field(2).(*array.StringBuilder)
	venueAt := builder.Field(3).(*array.TimestampBuilder)
	receivedAt := builder.Field(4).(*array.TimestampBuilder)
	price := builder.Field(5).(*array.Float64Builder)
	qty := builder.Field(6).(*array.Float64Builder)
	side := builder.Field(7).(*array.StringBuilder)
	ordType := builder.Field(8).(*array.StringBuilder)
	tradeID := builder.Field(9).(*array.Int64Builder)

	for _, row := range rows {
		epoch.Append(row.Epoch)
		tick.Append(row.Tick)
		text(symbol, row.Symbol)
		timestamp(venueAt, row.VenueAt)
		timestamp(receivedAt, row.ReceivedAt)
		price.Append(row.Price)
		qty.Append(row.Qty)
		text(side, row.Side)
		text(ordType, row.OrdType)
		tradeID.Append(row.TradeID)
	}
}

func fillExecutions(builder *array.RecordBuilder, rows []ExecutionRow) {
	epoch := builder.Field(0).(*array.Int64Builder)
	tick := builder.Field(1).(*array.Int64Builder)
	symbol := builder.Field(2).(*array.StringBuilder)
	venueAt := builder.Field(3).(*array.TimestampBuilder)
	receivedAt := builder.Field(4).(*array.TimestampBuilder)
	orderID := builder.Field(5).(*array.StringBuilder)
	orderUserRef := builder.Field(6).(*array.Int64Builder)
	execID := builder.Field(7).(*array.StringBuilder)
	execType := builder.Field(8).(*array.StringBuilder)
	tradeID := builder.Field(9).(*array.Int64Builder)
	side := builder.Field(10).(*array.StringBuilder)
	lastQty := builder.Field(11).(*array.Decimal128Builder)
	lastPrice := builder.Field(12).(*array.Decimal128Builder)
	liquidityInd := builder.Field(13).(*array.StringBuilder)
	cost := builder.Field(14).(*array.Decimal128Builder)
	orderType := builder.Field(15).(*array.StringBuilder)
	orderStatus := builder.Field(16).(*array.StringBuilder)
	cumQty := builder.Field(17).(*array.Decimal128Builder)
	cumCost := builder.Field(18).(*array.Decimal128Builder)
	avgPrice := builder.Field(19).(*array.Decimal128Builder)
	feeUsdEquiv := builder.Field(20).(*array.Decimal128Builder)
	fees := builder.Field(21).(*array.StringBuilder)

	for _, row := range rows {
		epoch.Append(row.Epoch)
		tick.Append(row.Tick)
		text(symbol, row.Symbol)
		timestamp(venueAt, row.VenueAt)
		timestamp(receivedAt, row.ReceivedAt)
		text(orderID, row.OrderID)
		orderUserRef.Append(row.OrderUserRef)
		text(execID, row.ExecID)
		text(execType, row.ExecType)
		tradeID.Append(row.TradeID)
		text(side, row.Side)
		money(lastQty, row.LastQty)
		money(lastPrice, row.LastPrice)
		text(liquidityInd, row.LiquidityInd)
		money(cost, row.Cost)
		text(orderType, row.OrderType)
		text(orderStatus, row.OrderStatus)
		money(cumQty, row.CumQty)
		money(cumCost, row.CumCost)
		money(avgPrice, row.AvgPrice)
		money(feeUsdEquiv, row.FeeUsdEquiv)
		text(fees, row.Fees)
	}
}

func fillMeasurements(builder *array.RecordBuilder, rows []MeasurementRow) {
	epoch := builder.Field(0).(*array.Int64Builder)
	tick := builder.Field(1).(*array.Int64Builder)
	source := builder.Field(2).(*array.StringBuilder)
	symbol := builder.Field(3).(*array.StringBuilder)
	venueAt := builder.Field(4).(*array.TimestampBuilder)
	observedAt := builder.Field(5).(*array.TimestampBuilder)
	maturity := builder.Field(6).(*array.Float64Builder)
	snr := builder.Field(7).(*array.Float64Builder)
	snrDefined := builder.Field(8).(*array.BooleanBuilder)
	metrics := builder.Field(9).(*array.MapBuilder)
	metadata := builder.Field(10).(*array.MapBuilder)
	payload := builder.Field(11).(*array.BinaryBuilder)

	metricsKey := metrics.KeyBuilder().(*array.StringBuilder)
	metricsVal := metrics.ItemBuilder().(*array.Float64Builder)

	metadataKey := metadata.KeyBuilder().(*array.StringBuilder)
	metadataVal := metadata.ItemBuilder().(*array.Float64Builder)

	for _, row := range rows {
		epoch.Append(row.Epoch)
		tick.Append(row.Tick)
		text(source, row.Source)
		text(symbol, row.Symbol)
		timestamp(venueAt, row.VenueAt)
		timestamp(observedAt, row.ObservedAt)
		maturity.Append(row.Maturity)

		if row.SNRDefined {
			snr.Append(row.SNR)
		} else {
			snr.AppendNull()
		}

		snrDefined.Append(row.SNRDefined)

		if len(row.Metrics) == 0 {
			metrics.AppendNull()
		} else {
			metrics.Append(true)
			for k, v := range row.Metrics {
				metricsKey.Append(k)
				metricsVal.Append(v)
			}
		}

		if len(row.Metadata) == 0 {
			metadata.AppendNull()
		} else {
			metadata.Append(true)
			for k, v := range row.Metadata {
				metadataKey.Append(k)
				metadataVal.Append(v)
			}
		}

		if len(row.Payload) == 0 {
			payload.AppendNull()
		} else {
			payload.Append(row.Payload)
		}
	}
}

func fillModels(builder *array.RecordBuilder, rows []ModelRow) {
	epoch := builder.Field(0).(*array.Int64Builder)
	tick := builder.Field(1).(*array.Int64Builder)
	agentID := builder.Field(2).(*array.Int32Builder)
	stepCount := builder.Field(3).(*array.Int64Builder)
	nodesCount := builder.Field(4).(*array.Int64Builder)
	payload := builder.Field(5).(*array.BinaryBuilder)

	for _, row := range rows {
		epoch.Append(row.Epoch)
		tick.Append(row.Tick)
		agentID.Append(row.AgentID)
		stepCount.Append(row.StepCount)
		nodesCount.Append(row.NodesCount)
		payload.Append(row.Payload)
	}
}

func fillGrids(builder *array.RecordBuilder, rows []GridRow) {
	epoch := builder.Field(0).(*array.Int64Builder)
	tick := builder.Field(1).(*array.Int64Builder)
	agentID := builder.Field(2).(*array.Int32Builder)
	contextLabel := builder.Field(3).(*array.StringBuilder)
	payload := builder.Field(4).(*array.BinaryBuilder)

	for _, row := range rows {
		epoch.Append(row.Epoch)
		tick.Append(row.Tick)
		agentID.Append(row.AgentID)
		text(contextLabel, row.ContextLabel)
		payload.Append(row.Payload)
	}
}

func fillPositions(builder *array.RecordBuilder, rows []PositionRow) {
	epoch := builder.Field(0).(*array.Int64Builder)
	tick := builder.Field(1).(*array.Int64Builder)
	symbol := builder.Field(2).(*array.StringBuilder)
	status := builder.Field(3).(*array.StringBuilder)
	qty := builder.Field(4).(*array.Decimal128Builder)
	basis := builder.Field(5).(*array.Decimal128Builder)
	entryPrice := builder.Field(6).(*array.Decimal128Builder)
	entryFee := builder.Field(7).(*array.Decimal128Builder)
	exitPrice := builder.Field(8).(*array.Decimal128Builder)
	exitFee := builder.Field(9).(*array.Decimal128Builder)
	mark := builder.Field(10).(*array.Decimal128Builder)
	pnl := builder.Field(11).(*array.Decimal128Builder)
	realizedPnL := builder.Field(12).(*array.Decimal128Builder)
	entryAt := builder.Field(13).(*array.TimestampBuilder)
	exitAt := builder.Field(14).(*array.TimestampBuilder)

	for _, row := range rows {
		epoch.Append(row.Epoch)
		tick.Append(row.Tick)
		text(symbol, row.Symbol)
		text(status, row.Status)
		money(qty, row.Qty)
		money(basis, row.Basis)
		money(entryPrice, row.EntryPrice)
		money(entryFee, row.EntryFee)
		money(exitPrice, row.ExitPrice)
		money(exitFee, row.ExitFee)
		money(mark, row.Mark)
		money(pnl, row.PnL)
		money(realizedPnL, row.RealizedPnL)

		if row.EntryAt != nil {
			timestamp(entryAt, *row.EntryAt)
		} else {
			entryAt.AppendNull()
		}

		if row.ExitAt != nil {
			timestamp(exitAt, *row.ExitAt)
		} else {
			exitAt.AppendNull()
		}
	}
}

func fillOutcomes(builder *array.RecordBuilder, rows []OutcomeRow) {
	epoch := builder.Field(0).(*array.Int64Builder)
	tick := builder.Field(1).(*array.Int64Builder)
	decisionID := builder.Field(2).(*array.Int64Builder)
	symbol := builder.Field(3).(*array.StringBuilder)
	at := builder.Field(4).(*array.TimestampBuilder)
	actionKind := builder.Field(5).(*array.StringBuilder)
	actionPower := builder.Field(6).(*array.Int32Builder)
	actionReduce := builder.Field(7).(*array.BooleanBuilder)
	authority := builder.Field(8).(*array.Float64Builder)
	outcome := builder.Field(9).(*array.Float64Builder)

	for _, row := range rows {
		epoch.Append(row.Epoch)
		tick.Append(row.Tick)
		decisionID.Append(row.DecisionID)
		text(symbol, row.Symbol)
		timestamp(at, row.At)
		text(actionKind, row.ActionKind)
		actionPower.Append(row.ActionPower)
		actionReduce.Append(row.ActionReduce)
		authority.Append(row.Authority)

		if row.Outcome != nil {
			outcome.Append(*row.Outcome)
		} else {
			outcome.AppendNull()
		}
	}
}
