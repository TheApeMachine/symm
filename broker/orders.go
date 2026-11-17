package broker

import (
	"slices"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/network"
	"github.com/theapemachine/symm/system"
	"github.com/theapemachine/symm/venue/kraken"
)

/*
Orders loads QueryOrders rows for fill order ids and sets them on the
holdings the ledger restored.
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
Restore queries each fill's order id and sets EntryOrder or ExitOrder.
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
			position := owners[id]

			if position == nil {
				continue
			}

			req := order.Request()

			if id == position.EntryVenueOrderID {
				position.EntryOrder = req
			}

			if id == position.ExitVenueOrderID {
				position.ExitOrder = req
			}
		}
	}
}
