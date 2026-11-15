package network

import (
	"context"
	"sync"
	"time"

	gorilla "github.com/gorilla/websocket"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/nomagique/runtime"
	"github.com/theapemachine/symm/system"
)

// Pinger writes websocket ping controls on a live conn.
type Pinger struct {
	*runtime.System
	conn    *gorilla.Conn
	writeMu *sync.Mutex
	once    sync.Once
}

func NewPinger(ctx context.Context) *Pinger {
	pinger := &Pinger{}
	pinger.System = runtime.NewSystem(ctx, "network.pinger", pinger)
	return pinger
}

func (pinger *Pinger) Start(
	conn *gorilla.Conn, writeMu *sync.Mutex, status *runtime.Stage,
) {
	pinger.writeMu = writeMu

	writeMu.Lock()
	pinger.conn = conn
	writeMu.Unlock()

	pinger.Transition(runtime.READY)

	pinger.once.Do(func() {
		go func() {
			ticker := time.NewTicker(system.Cfg.WebSocket.PingInterval)
			defer ticker.Stop()

			for {
				select {
				case <-pinger.Context().Done():
					return
				case <-ticker.C:
					pinger.writeMu.Lock()

					if pinger.conn != nil && pinger.Status() == runtime.READY {
						errnie.Error(pinger.conn.WriteControl(
							gorilla.PingMessage,
							nil,
							time.Now().Add(5*time.Second),
						))
					}

					pinger.writeMu.Unlock()
				}
			}
		}()
	})
}
