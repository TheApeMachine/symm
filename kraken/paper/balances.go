package paper

import (
	"github.com/bytedance/sonic"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/kraken/public"
	"github.com/theapemachine/symm/kraken/user"
)

func (ws *WebSocket) handleOutbound(value any) {
	payload, err := sonic.Marshal(value)

	if err != nil {
		errnie.Error(err)

		return
	}

	var frame struct {
		Method string `json:"method"`
		Params struct {
			Channel string `json:"channel"`
		} `json:"params"`
	}

	if err := sonic.Unmarshal(payload, &frame); err != nil {
		errnie.Error(err)

		return
	}

	if frame.Method != "subscribe" || frame.Params.Channel != public.BalancesChannel {
		return
	}

	ws.publishBalances("snapshot", ws.wallet.Snapshot())
}

func (ws *WebSocket) publishBalances(kind string, rows []user.Balance) {
	if len(rows) == 0 {
		return
	}

	ws.sequence++

	payload, err := sonic.Marshal(map[string]any{
		"channel":  public.BalancesChannel,
		"type":     kind,
		"data":     rows,
		"sequence": ws.sequence,
	})

	if err != nil {
		errnie.Error(err)

		return
	}

	var message public.SocketMessage

	if err := sonic.Unmarshal(payload, &message); err != nil {
		errnie.Error(err)

		return
	}

	ws.broadcasts["balances"].Send(&qpool.QValue[any]{Value: message})
}
