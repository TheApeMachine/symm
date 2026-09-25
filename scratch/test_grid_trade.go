package main

import (
	"context"
	"fmt"

	_ "capnproto.org/go/capnp/v3"
	"github.com/bytedance/sonic"
	"github.com/theapemachine/symm/nomagique/store"
)

func main() {
	grid := store.NewGrid(context.Background())

	interests := "ticker.capture.receivedAt,ticker.data.last,trade.data.qty,trade.data.side=sell,trade.data.price,trade.data.timestamp,level3.data.ask,level3.data.bid,level3.data.ask_qty,level3.data.bid_qty,ticker.data.index,ticker.data.mark,ticker.data.openInterest,ticker.data.ask,ticker.data.bid,ticker.data.ask_qty,ticker.data.bid_qty,level3.data.bids,level3.data.asks,level3.capture.receivedAt,futures_ticker.capture.receivedAt,futures_ticker.data.last,futures_ticker.data.index,futures_ticker.data.mark,futures_ticker.data.openInterest,held:ticker.data.last,futures_trade.data.price,futures_trade.data.qty,futures_trade.data.side=sell,futures_trade.data.type=liquidation,futures_trade.data.timestamp"

	tradeRecord := map[string]any{
		"channel":     "trade",
		"type":        "update",
		"_connection": 1,
		"_focus":      "BTC/USD",
		"data": map[string]any{
			"symbol":    "BTC/USD",
			"side":      "buy",
			"price":     84500.5,
			"qty":       0.1234,
			"timestamp": "2026-09-25T13:00:00.123456789Z",
		},
	}

	payload, _ := sonic.Marshal(tradeRecord)

	client := store.Grid_ServerToClient(grid)
	err := client.Write(context.Background(), func(p store.Grid_write_Params) error {
		_ = p.SetInterests(interests)
		dataList, _ := p.NewData(1)
		_ = dataList.Set(0, payload)
		return nil
	})
	if err != nil {
		fmt.Printf("Write failed: %v\n", err)
		return
	}

	ans, release := client.Done(context.Background(), func(p store.Grid_done_Params) error { return nil })
	defer release()

	res, err := ans.Struct()
	if err != nil {
		fmt.Printf("Grid.Done failed: %v\n", err)
		return
	}

	gridRes := store.Grid_done_Results(res)
	vals, _ := gridRes.Values()
	pres, _ := gridRes.Present()
	scope, _ := gridRes.Scope()

	fmt.Printf("Grid.Done: delivered=%d, scope=%q, values.Len=%d, pres.Len=%d\n",
		gridRes.Delivered(), scope, vals.Len(), pres.Len())

	for i := 0; i < vals.Len(); i++ {
		if pres.At(i) {
			fmt.Printf("  slot %d: val=%v\n", i, vals.At(i))
		}
	}
}
