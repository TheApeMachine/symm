package broker

import (
	"context"

	"github.com/bytedance/sonic"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/network"
)

/*
SubscribeExecutions opens the private executions stream. Live sends the
websocket token Kraken requires; paper uses the same subscribe envelope
through the simulator CLI.
*/
func SubscribeExecutions(ctx context.Context, transport Transport, instrument *Instrument) error {
	if transport == nil {
		return nil
	}

	params := map[string]any{
		"channel": "executions",
	}

	if _, live := transport.(*network.WebsocketClient); live {
		if instrument == nil {
			return errnie.Err(
				errnie.Validation,
				"[broker] executions subscribe needs instrument",
				nil,
			)
		}

		token, err := instrument.WebSocketToken(ctx)
		if err != nil {
			return err
		}

		params["token"] = token
		params["snap_orders"] = true
		params["snap_trades"] = false
	}

	buf, err := sonic.Marshal(map[string]any{
		"method": "subscribe",
		"params": params,
	})

	if err != nil {
		return errnie.Err(
			errnie.IO,
			"[broker] executions subscribe marshal failed",
			err,
		)
	}

	if err := transport.Write(buf); err != nil {
		return errnie.Err(
			errnie.IO,
			"[broker] executions subscribe write failed",
			err,
		)
	}

	return nil
}
