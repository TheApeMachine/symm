package data

import (
	"bytes"
	"context"

	"github.com/bytedance/sonic"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/runtime"
)

/*
CanonicalizeFuturesServer translates raw Kraken Futures websocket frames (ticker and trade)
into the standard channel/data format expected by the downstream signal graph.
*/
type CanonicalizeFuturesServer struct {
	*runtime.System
	out []byte
}

func NewCanonicalizeFutures(ctx context.Context) *CanonicalizeFuturesServer {
	server := &CanonicalizeFuturesServer{
		System: runtime.NewSystem(ctx, "data.canonicalize_futures"),
	}

	server.Transition(runtime.READY)
	return server
}

func (server *CanonicalizeFuturesServer) Write(ctx context.Context, call CanonicalizeFutures_write) error {
	dataBytes, err := call.Args().Data()
	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.BadRequest,
			"[data.canonicalize_futures.Write] failed to read data",
			err,
		))
	}

	server.out = nil
	if len(dataBytes) == 0 {
		return nil
	}

	var raw map[string]any
	if err := sonic.Unmarshal(dataBytes, &raw); err != nil {
		// Not JSON or non-object; pass through
		server.out = bytes.Clone(dataBytes)
		return nil
	}

	feed, hasFeed := raw["feed"].(string)
	if !hasFeed {
		// Already standard or non-futures message; pass through
		server.out = bytes.Clone(dataBytes)
		return nil
	}

	productID, _ := raw["product_id"].(string)

	if feed == "ticker" || feed == "ticker_lite" {
		dataMap := map[string]any{
			"symbol": productID,
		}
		if v, ok := raw["last"]; ok {
			dataMap["last"] = v
		}
		if v, ok := raw["bid"]; ok {
			dataMap["bid"] = v
		}
		if v, ok := raw["ask"]; ok {
			dataMap["ask"] = v
		}
		if v, ok := raw["bid_size"]; ok {
			dataMap["bid_qty"] = v
		}
		if v, ok := raw["ask_size"]; ok {
			dataMap["ask_qty"] = v
		}
		if v, ok := raw["index"]; ok {
			dataMap["index"] = v
		}
		if v, ok := raw["markPrice"]; ok {
			dataMap["mark"] = v
		}
		if v, ok := raw["openInterest"]; ok {
			dataMap["openInterest"] = v
		}
		if v, ok := raw["time"]; ok {
			dataMap["timestamp"] = v
		}

		outDoc := map[string]any{
			"channel": "ticker",
			"data":    dataMap,
		}

		encoded, err := sonic.Marshal(outDoc)
		if err != nil {
			return errnie.Error(errnie.Err(
				errnie.Internal,
				"[data.canonicalize_futures.Write] failed to marshal canonical ticker",
				err,
			))
		}
		server.out = encoded
		return nil
	}

	if feed == "trade" {
		dataMap := map[string]any{
			"symbol": productID,
		}
		if v, ok := raw["price"]; ok {
			dataMap["price"] = v
		}
		if v, ok := raw["qty"]; ok {
			dataMap["qty"] = v
		}
		if v, ok := raw["side"]; ok {
			dataMap["side"] = v
		}
		if v, ok := raw["time"]; ok {
			dataMap["timestamp"] = v
		}

		outDoc := map[string]any{
			"channel": "trade",
			"data":    dataMap,
		}

		encoded, err := sonic.Marshal(outDoc)
		if err != nil {
			return errnie.Error(errnie.Err(
				errnie.Internal,
				"[data.canonicalize_futures.Write] failed to marshal canonical trade",
				err,
			))
		}
		server.out = encoded
		return nil
	}

	if feed == "trade_snapshot" {
		if trades, ok := raw["trades"].([]any); ok && len(trades) > 0 {
			if lastTrade, ok := trades[len(trades)-1].(map[string]any); ok {
				dataMap := map[string]any{
					"symbol": productID,
				}
				if v, ok := lastTrade["price"]; ok {
					dataMap["price"] = v
				}
				if v, ok := lastTrade["qty"]; ok {
					dataMap["qty"] = v
				}
				if v, ok := lastTrade["side"]; ok {
					dataMap["side"] = v
				}
				if v, ok := lastTrade["time"]; ok {
					dataMap["timestamp"] = v
				}

				outDoc := map[string]any{
					"channel": "trade",
					"data":    dataMap,
				}

				encoded, err := sonic.Marshal(outDoc)
				if err != nil {
					return errnie.Error(errnie.Err(
						errnie.Internal,
						"[data.canonicalize_futures.Write] failed to marshal canonical trade snapshot",
						err,
					))
				}
				server.out = encoded
				return nil
			}
		}
	}

	// Fallback to raw bytes
	server.out = bytes.Clone(dataBytes)
	return nil
}

func (server *CanonicalizeFuturesServer) Done(ctx context.Context, call CanonicalizeFutures_done) error {
	results, err := call.AllocResults()
	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"[data.canonicalize_futures.Done] failed to allocate results",
			err,
		))
	}

	results.SetStatus(runtime.Status(server.Status()))
	if len(server.out) > 0 {
		if err := results.SetOut(server.out); err != nil {
			return errnie.Error(errnie.Err(
				errnie.Internal,
				"[data.canonicalize_futures.Done] failed to set out",
				err,
			))
		}
	}

	return nil
}
