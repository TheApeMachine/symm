package broker

import (
	"context"

	"github.com/theapemachine/symm/network"

	"github.com/theapemachine/symm/nomagique/runtime"
)

type Desk struct {
	*runtime.System
	private   *network.WebsocketClient
	price     *Price
	balance   *Balance
	Execution *Execution
}

func NewDesk(
	ctx context.Context,
	private *network.WebsocketClient,
	price *Price,
	balance *Balance,
) *Desk {
	desk := &Desk{
		private:   private,
		price:     price,
		balance:   balance,
		Execution: NewExecution(ctx, private, price, balance),
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
