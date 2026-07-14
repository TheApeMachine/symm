package websocket

import (
	"github.com/theapemachine/datura"
	"github.com/theapemachine/symm/kraken"
)

/*
Lifecycle emits paper order and execution frames through the latency simulator.
*/
type Lifecycle struct {
	paper *Paper
}

func NewLifecycle(paper *Paper) *Lifecycle {
	return &Lifecycle{paper: paper}
}

func (lifecycle *Lifecycle) Balance() error {
	var model datura.Map[any]
	var err error

	lifecycle.paper.simulator.Do(REST, func() {
		model, err = lifecycle.paper.execute("balances", "balance")
	})

	if err != nil {
		return err
	}

	return lifecycle.paper.simulator.Emit(
		lifecycle.paper, WEBSOCKET, "balances", kraken.NewBalanceFromMap(model),
	)
}

func (lifecycle *Lifecycle) Replay(trades []any) error {
	for tradeIndex, tradeRaw := range trades {
		trade, ok := tradeRaw.(map[string]any)

		if !ok {
			continue
		}

		execution := kraken.NewExecutionFromMap(datura.Map[any](trade))

		if tradeIndex == 0 {
			execution.Type = "snapshot"
		}

		err := lifecycle.paper.simulator.Emit(lifecycle.paper, WEBSOCKET, "executions", execution)

		if err != nil {
			return err
		}
	}

	return nil
}

func (lifecycle *Lifecycle) Place(model datura.Map[any], reqID int) error {
	orderAck := kraken.NewOrderResponseFromMap(model, reqID)

	err := lifecycle.paper.simulator.Emit(lifecycle.paper, WEBSOCKET, "add_order", orderAck)

	if err != nil {
		return err
	}

	err = lifecycle.paper.simulator.Emit(
		lifecycle.paper, WEBSOCKET, "executions", kraken.NewExecutionFromMap(model),
	)

	if err != nil {
		return err
	}

	return lifecycle.Balance()
}
