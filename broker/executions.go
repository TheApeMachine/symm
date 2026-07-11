package broker

import (
	"bytes"
	"slices"
	"strings"

	"github.com/theapemachine/datura"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/types"
)

type Executions struct {
	desk *Desk
	ui   chan []byte
}

func NewExecutions(desk *Desk, ui chan []byte) *Executions {
	return &Executions{
		desk: desk,
		ui:   ui,
	}
}

func (executions *Executions) On(data []byte) {
	updateType := "update"
	rows := kraken.NewExecutionDataSlice(data)
	raw := bytes.TrimSpace(data)

	if len(raw) > 0 && raw[0] == '{' {
		frame := kraken.NewExecution(data)

		if frame.Channel == "executions" {
			updateType = frame.Type
			slice := kraken.ExecutionDataSlice(frame.Data)
			rows = &slice
		}
	}

	executions.Measure(updateType, rows)
	positions := make([]PositionData, 0)

	executions.desk.positions.Range(func(key any, value any) bool {
		position := value.(*Position)

		if slices.Contains(
			[]types.Status{types.CLOSED, types.FATAL}, position.status,
		) {
			executions.desk.releasePosition(key.(string), position)
			return true
		}

		positions = append(positions, *position.data)
		return true
	})

	executions.desk.refreshStatus()

	if executions.ui == nil {
		return
	}

	if len(*rows) > 0 {
		select {
		case executions.ui <- datura.Map[any]{
			"executions": []kraken.ExecutionDataSlice{*rows},
		}.Marshal():
		default:
		}
	}

	select {
	case executions.ui <- datura.Map[any]{
		"positions": positions,
	}.Marshal():
	default:
	}
}

func (executions *Executions) Measure(updateType string, slice *kraken.ExecutionDataSlice) {
	snapshot := len(*slice) == 0 || strings.EqualFold(updateType, "snapshot")
	snapshotSymbols := map[string]struct{}{}

	for _, execution := range *slice {
		symbol := strings.TrimSpace(execution.Symbol)

		if symbol == "" {
			errnie.Error(errnie.Err(
				errnie.Validation,
				"broker: execution missing symbol",
				nil,
			))
			continue
		}

		qty := execution.CumQty
		if qty <= 0 {
			qty = execution.LastQty
		}

		// Determine if this execution represents an active/open position
		isActiveOrder := strings.EqualFold(execution.OrderStatus, "open") || strings.EqualFold(execution.OrderStatus, "partially_filled")
		isSnapshotPosition := strings.EqualFold(updateType, "snapshot") && qty > 0 && strings.EqualFold(execution.OrderStatus, "filled") && strings.EqualFold(execution.Side, "buy")

		// Also respect paper emulator's explicit PositionStatus for folded balances
		isPaperPosition := strings.EqualFold(execution.PositionStatus, "open")

		if isActiveOrder || isSnapshotPosition || isPaperPosition {
			snapshotSymbols[symbol] = struct{}{}
		}

		position, ok := executions.desk.positions.Load(symbol)

		if ok {
			held := position.(*Position)

			if err := held.Execution(&execution); err != nil {
				errnie.Error(err)
			}

			continue
		}

		if qty <= 0 || execution.AvgPrice.Rat().Sign() <= 0 {
			continue
		}

		if !isActiveOrder && !isSnapshotPosition && !isPaperPosition {
			continue
		}

		feeRate, feeFound := executions.desk.takerRate(symbol)

		if !feeFound {
			errnie.Error(errnie.Err(
				errnie.NotFound,
				"broker: TradeVolume taker fee missing for "+symbol,
				nil,
			))
			continue
		}

		position, _ = executions.desk.positions.LoadOrStore(symbol, NewPosition(
			executions.desk.private,
			&PositionData{
				Symbol:     symbol,
				Qty:        qty,
				EntryPrice: execution.AvgPrice,
				Mark:       execution.AvgPrice,
			},
		))

		// Make sure to set the underlying position quantities correctly upon instantiation
		// if they somehow drift.
		pos := position.(*Position)
		if pos.data.Qty <= 0 {
			pos.data.Qty = qty
		}
		if pos.data.EntryPrice.Rat().Sign() <= 0 {
			pos.data.EntryPrice = execution.AvgPrice
		}

		pos.SetFeeRate(feeRate)
		pos.executions = []*kraken.ExecutionData{&execution}
		pos.status = types.OPEN
		pos.exposed = true

		executions.desk.refreshStatus()
	}

	if snapshot {
		executions.desk.positions.Range(func(key any, value any) bool {
			symbol := key.(string)

			if _, ok := snapshotSymbols[symbol]; ok {
				return true
			}

			position := value.(*Position)

			if position.status == types.PENDING && !position.closing {
				return true
			}

			position.status = types.CLOSED
			return true
		})
	}
}
