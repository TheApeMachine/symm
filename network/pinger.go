package network

import (
	"context"
	"fmt"
	"time"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/runtime"
	"github.com/theapemachine/symm/system"
)

type Pinger struct {
	*runtime.System
	client *WebsocketClient
	stop   chan struct{}
}

func NewPinger(ctx context.Context, client *WebsocketClient) *Pinger {
	pinger := &Pinger{client: client}
	pinger.System = runtime.NewSystem(ctx, "network.pinger", pinger)
	return pinger
}

func (pinger *Pinger) Start() {
	if pinger.stop != nil {
		close(pinger.stop)
		pinger.stop = nil
	}

	stop := make(chan struct{})
	pinger.stop = stop
	pinger.Transition(runtime.READY)

	go pinger.loop(stop)
}

func (pinger *Pinger) Stop() {
	if pinger.stop == nil {
		return
	}

	close(pinger.stop)
	pinger.stop = nil

	if pinger.Status() == runtime.READY {
		pinger.Transition(runtime.WAITING)
	}
}

func (pinger *Pinger) loop(stop <-chan struct{}) {
	interval := system.Cfg.WebSocket.PingInterval

	// system.websocket.ping_interval has its default in system; a
	// non-positive value is a broken config, not a reason to pick another.
	if interval <= 0 {
		pinger.Error(errnie.Err(
			errnie.Validation,
			fmt.Sprintf("[network.pinger] system.websocket.ping_interval %s is not positive", interval),
			nil,
		))

		return
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-stop:
			return
		case <-pinger.Context().Done():
			return
		case <-ticker.C:
			if pinger.Status() != runtime.READY {
				continue
			}

			// Soft-fail: writePing drops the client to WAITING for reconnect
			// and SoftDown warns; System.Error would flood ERROR and kill
			// the session.
			if err := pinger.client.writePing(); err != nil {
				errnie.Warn("[network.pinger] ping failed, reconnecting: " + err.Error())
			}
		}
	}
}
