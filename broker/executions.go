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
	slice := kraken.NewExecutionDataSlice(data)
	executions.Measure(slice)

	positions := make([]*PositionData, 0)

	executions.desk.positions.Range(func(key any, value any) bool {
		position := value.(*Position)

		if slices.Contains(
			[]types.Status{types.CLOSED, types.FATAL}, position.status,
		) {
			executions.desk.positions.Delete(key)
			return true
		}

		positions = append(positions, position.data)
		return true
	})

	executions.desk.refreshStatus()

	if executions.ui == nil {
		return
	}

	if len(*slice) > 0 {
		executions.ui <- datura.Map[any]{
			"executions": []kraken.ExecutionDataSlice{*slice},
		}.Marshal()
	}

	executions.ui <- datura.Map[any]{
		"positions": positions,
	}.Marshal()
}

func (executions *Executions) Measure(slice *kraken.ExecutionDataSlice) {
	snapshot := len(*slice) == 0
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

		if strings.EqualFold(execution.ExecType, "snapshot") {
			snapshot = true
		}

		if strings.EqualFold(execution.PositionStatus, "open") {
			snapshotSymbols[symbol] = struct{}{}
		}

		position, ok := executions.desk.positions.Load(symbol)

		if ok {
			held := position.(*Position)
			held.Execution(&execution)
			continue
		}

		if !strings.EqualFold(execution.PositionStatus, "open") ||
			execution.LastQty <= 0 ||
			execution.AvgPrice.Rat().Sign() <= 0 {
			continue
		}

		position, _ = executions.desk.positions.LoadOrStore(symbol, NewPosition(
			executions.desk.private,
			&PositionData{
				Symbol:     symbol,
				Qty:        execution.LastQty,
				EntryPrice: execution.AvgPrice,
				Mark:       execution.AvgPrice,
			},
		))

		position.(*Position).SetFeeRate(executions.desk.takerRate(symbol))
		position.(*Position).executions = []*kraken.ExecutionData{&execution}
		position.(*Position).status = types.OPEN

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
