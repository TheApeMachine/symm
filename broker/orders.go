package broker

import (
	"slices"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/network"
	"github.com/theapemachine/symm/system"
	"github.com/theapemachine/symm/venue/kraken"
)

/*
Orders restores venue order rows onto positions after trades admit them.
Tier composes this — Cache only holds lanes.
*/
type Orders struct {
	transport network.Client
}

/*
NewOrders wires the private transport used for QueryOrders.
*/
func NewOrders(transport network.Client) *Orders {
	orders := &Orders{
		transport: transport,
	}

	if err := errnie.Require(map[string]any{
		"transport": transport,
	}); err != nil {
		errnie.Error(err)
		return nil
	}

	return orders
}

/*
Restore QueryOrders every fill OrderID and Bind as each batch returns.
*/
func (orders *Orders) Restore(positions []*Position) {
	owners := make(map[string]*Position)

	for _, position := range positions {
		if position == nil {
			continue
		}

		for _, fill := range position.Fills {
			if fill == nil || fill.OrderID == "" {
				continue
			}

			owners[fill.OrderID] = position
		}
	}

	if len(owners) == 0 {
		return
	}

	ids := make([]string, 0, len(owners))

	for id := range owners {
		ids = append(ids, id)
	}

	for chunk := range slices.Chunk(ids, 50) {
		raw, err := orders.transport.Post(
			system.Cfg.WebSocket.Endpoints.QueryOrders,
			kraken.NewQueryOrdersRequest(chunk),
		)

		if err != nil {
			errnie.Error(err)
			return
		}

		batch := kraken.NewQueryOrdersResult(raw)

		if batch == nil {
			return
		}

		for id, order := range batch {
			if position := owners[id]; position != nil {
				position.Bind(id, order.Descr.Type, order.Request())
			}
		}
	}
}
