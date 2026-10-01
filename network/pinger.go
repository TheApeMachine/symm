package network

import (
	"context"
	"sync"
	"time"

	"github.com/theapemachine/symm/nomagique/runtime"
	"github.com/theapemachine/symm/system"
)

type Pinger struct {
	*runtime.System
	client *WebsocketClient
	mu     sync.Mutex
	stop   chan struct{}
}

func NewPinger(ctx context.Context, client *WebsocketClient) *Pinger {
	pinger := &Pinger{client: client}
	pinger.System = runtime.NewSystem(ctx, "network.pinger", pinger)
	return pinger
}

func (pinger *Pinger) Start() {
	pinger.mu.Lock()
	defer pinger.mu.Unlock()

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
	pinger.mu.Lock()
	defer pinger.mu.Unlock()

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

	if interval <= 0 {
		interval = 10 * time.Second
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

			// Soft-fail: writePing drops the client to WAITING for reconnect.
			// Do not System.Error — that floods ERROR and kills the session.
			_ = pinger.client.writePing()
		}
	}
}
