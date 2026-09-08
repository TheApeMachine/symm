package tables

import (
	"github.com/apache/arrow-go/v18/arrow/array"
)

/*
The fill functions below drive an Arrow RecordBuilder from a slice of rows.
Column indices follow the field order of the matching schema, so a field added
to a schema must be added here at the same position.
*/

func fillRuns(builder *array.RecordBuilder, rows []RunRow) {
	id := builder.Field(0).(*array.StringBuilder)
	startedAt := builder.Field(1).(*array.TimestampBuilder)
	commit := builder.Field(2).(*array.StringBuilder)
	build := builder.Field(3).(*array.StringBuilder)
	digest := builder.Field(4).(*array.StringBuilder)
	integrity := builder.Field(5).(*array.StringBuilder)
	positions := builder.Field(6).(*array.Int32Builder)
	versions := builder.Field(7).(*array.MapBuilder)

	for _, row := range rows {
		id.Append(row.ID)
		timestamp(startedAt, row.StartedAt)
		text(commit, row.CodeCommit)
		text(build, row.BuildID)
		text(digest, row.ConfigDigest)
		integrity.Append(row.Integrity)
		positions.Append(row.Positions)

		if row.SchemaVersions == nil {
			versions.AppendNull()

			continue
		}

		versions.Append(true)
		keys := versions.KeyBuilder().(*array.StringBuilder)
		values := versions.ItemBuilder().(*array.StringBuilder)

		for key, value := range row.SchemaVersions {
			keys.Append(key)
			values.Append(value)
		}
	}
}

func fillCaptures(builder *array.RecordBuilder, rows []CaptureRow) {
	run := builder.Field(0).(*array.StringBuilder)
	sequence := builder.Field(1).(*array.Int64Builder)
	stream := builder.Field(2).(*array.StringBuilder)
	epoch := builder.Field(3).(*array.Int64Builder)
	streamSeq := builder.Field(4).(*array.Int64Builder)
	receivedAt := builder.Field(5).(*array.TimestampBuilder)
	endpoint := builder.Field(6).(*array.StringBuilder)
	kind := builder.Field(7).(*array.StringBuilder)
	hash := builder.Field(8).(*array.StringBuilder)
	payload := builder.Field(9).(*array.BinaryBuilder)

	for _, row := range rows {
		run.Append(row.Run)
		sequence.Append(row.Sequence)
		stream.Append(row.Stream)
		epoch.Append(row.StreamEpoch)
		streamSeq.Append(row.StreamSequence)
		timestamp(receivedAt, row.ReceivedAt)
		text(endpoint, row.Endpoint)
		kind.Append(row.Kind)
		hash.Append(row.PayloadHash)

		if row.Payload == nil {
			payload.AppendNull()

			continue
		}

		payload.Append(row.Payload)
	}
}

func fillManifests(builder *array.RecordBuilder, rows []ManifestRow) {
	run := builder.Field(0).(*array.StringBuilder)
	ref := builder.Field(1).(*array.StructBuilder)
	workload := builder.Field(2).(*array.StringBuilder)
	domain := builder.Field(3).(*array.StringBuilder)
	symbol := builder.Field(4).(*array.StringBuilder)
	venueAt := builder.Field(5).(*array.TimestampBuilder)
	venueSeq := builder.Field(6).(*array.StringBuilder)

	for _, row := range rows {
		run.Append(row.Run)
		envelope(ref, row.Envelope)
		workload.Append(row.Workload)
		text(domain, row.DomainKind)
		text(symbol, row.Symbol)
		timestamp(venueAt, row.VenueAt)
		text(venueSeq, row.VenueSequence)
	}
}

func fillWitnesses(builder *array.RecordBuilder, rows []WitnessRow) {
	run := builder.Field(0).(*array.StringBuilder)
	ref := builder.Field(1).(*array.StructBuilder)
	boundary := builder.Field(2).(*array.StringBuilder)
	artifactKind := builder.Field(3).(*array.StringBuilder)
	artifactID := builder.Field(4).(*array.StringBuilder)
	artifactLabel := builder.Field(5).(*array.StringBuilder)
	producedAt := builder.Field(6).(*array.TimestampBuilder)
	component := builder.Field(7).(*array.StringBuilder)
	version := builder.Field(8).(*array.Int64Builder)
	parents := builder.Field(9).(*array.ListBuilder)
	semantic := builder.Field(10).(*array.ListBuilder)
	payload := builder.Field(11).(*array.BinaryBuilder)

	parentValue := parents.ValueBuilder().(*array.StructBuilder)
	semanticValue := semantic.ValueBuilder().(*array.StringBuilder)

	for _, row := range rows {
		run.Append(row.Run)
		envelope(ref, row.Envelope)
		boundary.Append(row.Boundary)
		text(artifactKind, row.ArtifactKind)
		text(artifactID, row.ArtifactIdentity)
		text(artifactLabel, row.ArtifactKindLabel)
		timestamp(producedAt, row.ProducedAt)
		text(component, row.Component)
		version.Append(row.ComponentStateVersion)

		if row.ImmediateParents == nil {
			parents.AppendNull()
		} else {
			parents.Append(true)

			for _, parent := range row.ImmediateParents {
				envelope(parentValue, parent)
			}
		}

		if row.SemanticParents == nil {
			semantic.AppendNull()
		} else {
			semantic.Append(true)

			for _, name := range row.SemanticParents {
				semanticValue.Append(name)
			}
		}

		if row.Payload == nil {
			payload.AppendNull()

			continue
		}

		payload.Append(row.Payload)
	}
}

func fillLifecycle(builder *array.RecordBuilder, rows []LifecycleRow) {
	run := builder.Field(0).(*array.StringBuilder)
	decision := builder.Field(1).(*array.StringBuilder)
	correlation := builder.Field(2).(*array.StringBuilder)
	symbol := builder.Field(3).(*array.StringBuilder)
	kind := builder.Field(4).(*array.StringBuilder)
	action := builder.Field(5).(*array.StringBuilder)
	at := builder.Field(6).(*array.TimestampBuilder)
	captureSeq := builder.Field(7).(*array.Int64Builder)

	orderID := builder.Field(8).(*array.StringBuilder)
	clientOrderID := builder.Field(9).(*array.StringBuilder)
	execID := builder.Field(10).(*array.StringBuilder)
	execType := builder.Field(11).(*array.StringBuilder)
	tradeID := builder.Field(12).(*array.Int64Builder)
	side := builder.Field(13).(*array.StringBuilder)
	orderType := builder.Field(14).(*array.StringBuilder)
	orderStatus := builder.Field(15).(*array.StringBuilder)
	liquidity := builder.Field(16).(*array.StringBuilder)
	execAt := builder.Field(17).(*array.TimestampBuilder)
	lastQty := builder.Field(18).(*array.Decimal128Builder)
	lastPrice := builder.Field(19).(*array.Decimal128Builder)
	cost := builder.Field(20).(*array.Decimal128Builder)
	cumQty := builder.Field(21).(*array.Decimal128Builder)
	cumCost := builder.Field(22).(*array.Decimal128Builder)
	avgPrice := builder.Field(23).(*array.Decimal128Builder)
	feeUsd := builder.Field(24).(*array.Decimal128Builder)
	fees := builder.Field(25).(*array.StringBuilder)

	for _, row := range rows {
		run.Append(row.Run)
		decision.Append(row.DecisionID)
		text(correlation, row.ActionCorrelationID)
		symbol.Append(row.Symbol)
		kind.Append(row.Kind)
		text(action, row.Action)
		timestamp(at, row.At)
		captureSeq.Append(row.CaptureSeq)

		// Position open and close carry no execution fact, so every exec_
		// column is null rather than zero: a zero cost is a real economic
		// claim and these events do not make one.
		execution := row.Exec

		if execution == nil {
			execution = &ExecutionRow{}
		}

		text(orderID, execution.OrderID)
		text(clientOrderID, execution.ClientOrderID)
		text(execID, execution.ExecID)
		text(execType, execution.ExecType)

		if row.Exec == nil {
			tradeID.AppendNull()
		} else {
			tradeID.Append(execution.TradeID)
		}

		text(side, execution.Side)
		text(orderType, execution.OrderType)
		text(orderStatus, execution.OrderStatus)
		text(liquidity, execution.LiquidityInd)
		timestamp(execAt, execution.At)
		money(lastQty, execution.LastQty)
		money(lastPrice, execution.LastPrice)
		money(cost, execution.Cost)
		money(cumQty, execution.CumQty)
		money(cumCost, execution.CumCost)
		money(avgPrice, execution.AvgPrice)
		money(feeUsd, execution.FeeUsdEquiv)
		text(fees, execution.Fees)
	}
}

func fillOutcomes(builder *array.RecordBuilder, rows []OutcomeRow) {
	run := builder.Field(0).(*array.StringBuilder)
	decision := builder.Field(1).(*array.Int64Builder)
	trader := builder.Field(2).(*array.Int32Builder)
	label := builder.Field(3).(*array.StringBuilder)
	at := builder.Field(4).(*array.TimestampBuilder)
	actionKind := builder.Field(5).(*array.StringBuilder)
	actionPower := builder.Field(6).(*array.Int32Builder)
	actionReduce := builder.Field(7).(*array.BooleanBuilder)
	authority := builder.Field(8).(*array.Float64Builder)
	outcome := builder.Field(9).(*array.Float64Builder)
	context := builder.Field(10).(*array.ListBuilder)
	through := builder.Field(11).(*array.TimestampBuilder)
	value := builder.Field(12).(*array.Float64Builder)
	complete := builder.Field(13).(*array.BooleanBuilder)
	forced := builder.Field(14).(*array.BooleanBuilder)
	initial := builder.Field(15).(*array.Decimal128Builder)
	reference := builder.Field(16).(*array.Decimal128Builder)
	quantity := builder.Field(17).(*array.Decimal128Builder)
	cost := builder.Field(18).(*array.Decimal128Builder)
	fee := builder.Field(19).(*array.Decimal128Builder)
	opportunity := builder.Field(20).(*array.Decimal128Builder)

	contextValue := context.ValueBuilder().(*array.Int64Builder)

	for _, row := range rows {
		run.Append(row.Run)
		decision.Append(row.DecisionID)
		trader.Append(row.Trader)
		label.Append(row.Label)
		timestamp(at, row.At)
		actionKind.Append(row.ActionKind)
		actionPower.Append(row.ActionPower)
		actionReduce.Append(row.ActionReduce)
		authority.Append(row.Authority)

		if row.Outcome == nil {
			outcome.AppendNull()
		} else {
			outcome.Append(*row.Outcome)
		}

		if row.Context == nil {
			context.AppendNull()
		} else {
			context.Append(true)

			for _, token := range row.Context {
				contextValue.Append(token)
			}
		}

		timestamp(through, row.Through)
		value.Append(row.Value)
		complete.Append(row.Complete)
		forced.Append(row.Forced)
		money(initial, row.Initial)
		money(reference, row.Reference)
		money(quantity, row.Quantity)
		money(cost, row.Cost)
		money(fee, row.Fee)
		money(opportunity, row.Opportunity)
	}
}

func fillGaps(builder *array.RecordBuilder, rows []GapRow) {
	run := builder.Field(0).(*array.StringBuilder)
	sequence := builder.Field(1).(*array.Int64Builder)
	encoding := builder.Field(2).(*array.StringBuilder)

	for _, row := range rows {
		run.Append(row.Run)
		sequence.Append(row.Sequence)
		encoding.Append(row.Encoding)
	}
}
