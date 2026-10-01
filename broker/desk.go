package broker

import (
	"context"

	"github.com/theapemachine/symm/nomagique/runtime"
)

type Desk struct {
	*runtime.System
	transport Transport
	price     *Price
	balance   *Balance
	Execution *Execution
}

func NewDesk(
	ctx context.Context,
	transport Transport,
	price *Price,
	balance *Balance,
) *Desk {
	desk := &Desk{
		transport: transport,
		price:     price,
		balance:   balance,
		Execution: NewExecution(ctx, transport, price, balance),
	}

	desk.System = runtime.NewSystem(ctx, "desk", desk)

	if price != nil && price.Anomalies() != nil {
		price.Anomalies().SetOnFault(func(symbol string) {
			desk.Transition(runtime.ERROR)
		})

		price.Anomalies().SetOnRecover(func(symbol string) {
			if !price.Anomalies().HasAnySevereFault() {
				desk.Transition(runtime.READY)
			}
		})
	}

	return desk
}
