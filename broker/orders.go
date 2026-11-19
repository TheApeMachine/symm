package broker

import (
	"maps"
	"slices"
	"time"

	"github.com/shopspring/decimal"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/network"
	"github.com/theapemachine/symm/system"
	"github.com/theapemachine/symm/venue/kraken"
)

/*
Orders is QueryOrders: fetch venue order rows by txid so restore can bind
EntryOrder / ExitOrder the same way live enter/exit does — order first, fills after.
*/
type Orders struct {
	transport network.Client
}

/*
NewOrders wires the private transport used for QueryOrders.
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
openTm is the order's venue open time, or zero when the row has none.
*/
func openTm(order kraken.HistoryOrder) time.Time {
	if order.OpenTm == nil {
		return time.Time{}
	}

	// Stay in decimal — float64 loses sub-ms at ~1.7e9 epoch scale.
	whole := order.OpenTm.IntPart()
	frac := order.OpenTm.Sub(decimal.NewFromInt(whole))
	nsec := frac.Mul(decimal.NewFromInt(1e9)).IntPart()

	return time.Unix(whole, nsec).UTC()
}
