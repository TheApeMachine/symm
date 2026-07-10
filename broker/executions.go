package broker

import (
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
	frame := kraken.NewExecution(data)
	if frame.Channel != "executions" {
		// Just handle raw array or unsupported format for fallback
		slice := kraken.NewExecutionDataSlice(data)
		executions.Measure("update", slice)
	} else {
		slice := kraken.ExecutionDataSlice(frame.Data)
		executions.Measure(frame.Type, &slice)
	}

	positions := make([]PositionData, 0)

	executions.desk.positions.Range(func(key any, value any) bool {
		position := value.(*Position)

		if slices.Contains(
			[]types.Status{types.CLOSED, types.FATAL}, position.status,
		) {
			executions.desk.positions.Delete(key)
			return true
		}

		positions = append(positions, *position.data)
		return true
	})

	executions.desk.refreshStatus()

	if executions.ui == nil {
		return
	}

	// Assuming frame contains valid data for UI dispatch
	if executions.ui != nil {
		slice := kraken.ExecutionDataSlice(frame.Data)
		if len(slice) > 0 {
			select {
			case executions.ui <- datura.Map[any]{
				"executions": []kraken.ExecutionDataSlice{slice},
			}.Marshal():
			default:
			}
		}
	}

	if executions.ui != nil {
		select {
		case executions.ui <- datura.Map[any]{
			"positions": positions,
		}.Marshal():
		default:
		}
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
			held.Execution(&execution)
			continue
		}

		if qty <= 0 || execution.AvgPrice.Rat().Sign() <= 0 {
			continue
		}

		if !isActiveOrder && !isSnapshotPosition && !isPaperPosition {
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

		pos.SetFeeRate(executions.desk.takerRate(symbol))
		pos.executions = []*kraken.ExecutionData{&execution}
		pos.status = types.OPEN

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
