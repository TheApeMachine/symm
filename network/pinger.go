package network

import (
	"context"
	"time"

	"github.com/gorilla/websocket"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/runtime"
	"github.com/theapemachine/symm/system"
)

type Pinger struct {
	*runtime.System
	client *WebsocketClient
}

func NewPinger(ctx context.Context, client *WebsocketClient) *Pinger {
	pinger := &Pinger{client: client}
	pinger.System = runtime.NewSystem(ctx, "network.pinger", pinger)
	return pinger
}

func (pinger *Pinger) Start() {
	pinger.Transition(runtime.READY)

	go func() {
		interval := system.Cfg.WebSocket.PingInterval

		if interval <= 0 {
			interval = 10 * time.Second
		}

		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				if pinger.Status() != runtime.READY {
					continue
				}

				if err := pinger.client.conn.WriteMessage(
					websocket.PingMessage, nil,
				); err != nil {
					pinger.Error(errnie.Err(
						errnie.IO,
						"network.pinger: ping failed",
						err,
					))

					pinger.Transition(runtime.ERROR)
					return
				}
			case <-pinger.Context().Done():
				return
			}
		}
	}()
}
