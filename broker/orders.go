package broker

import (
	"maps"
	"slices"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/network"
	"github.com/theapemachine/symm/system"
	"github.com/theapemachine/symm/venue/kraken"
)

/*
Orders is QueryOrders / OpenOrders / CancelOrder against the private transport
so restore can bind EntryOrder and clear resting sells that would block Exit.
*/
type Orders struct {
	transport network.Client
}

/*
NewOrders wires the private transport used for order REST.
*/
func NewOrders(transport network.Client) *Orders {
	if transport == nil {
		return nil
	}

	return &Orders{transport: transport}
}

/*
Fetch loads HistoryOrder rows for the given txids (chunks of 50). Missing or
failed pages leave those ids absent — callers still fold from fill sides.
*/
func (orders *Orders) Fetch(ids []string) map[string]kraken.HistoryOrder {
	out := make(map[string]kraken.HistoryOrder, len(ids))

	if orders == nil || orders.transport == nil || len(ids) == 0 {
		return out
	}

	if system.Cfg == nil || system.Cfg.WebSocket == nil {
		return out
	}

	for chunk := range slices.Chunk(ids, 50) {
		raw, err := orders.transport.Post(
			system.Cfg.WebSocket.Endpoints.QueryOrders,
			kraken.NewQueryOrdersRequest(chunk),
		)

		if err != nil {
			errnie.Error(err)
			return out
		}

		batch := kraken.NewQueryOrdersResult(raw)

		if batch == nil {
			return out
		}

		maps.Copy(out, batch)
	}

	return out
}

/*
ListOpen loads resting venue orders. Nil/empty on transport failure.
*/
func (orders *Orders) ListOpen() map[string]kraken.HistoryOrder {
	if orders == nil || orders.transport == nil {
		return nil
	}

	if system.Cfg == nil || system.Cfg.WebSocket == nil {
		return nil
	}

	raw, err := orders.transport.Post(
		system.Cfg.WebSocket.Endpoints.OpenOrders,
		map[string]any{},
	)

	if err != nil {
		errnie.Error(err)
		return nil
	}

	return kraken.NewOpenOrdersResult(raw)
}

/*
Cancel cancels one resting venue order by txid.
*/
func (orders *Orders) Cancel(txid string) bool {
	if orders == nil || orders.transport == nil || txid == "" {
		return false
	}

	if system.Cfg == nil || system.Cfg.WebSocket == nil {
		return false
	}

	raw, err := orders.transport.Post(
		system.Cfg.WebSocket.Endpoints.CancelOrder,
		kraken.NewCancelOrderRequest(txid),
	)

	if err != nil {
		errnie.Error(err)
		return false
	}

	return kraken.CancelOrderResult(raw)
}
