package websocket

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/system"
)

/*
Pinger owns one session's keepalive loop. Spot and futures send different
payloads — spot a {"method":"ping"} message, futures a protocol ping control
frame, since Kraken Futures rejects an application-level ping — so the session
supplies the send and the Pinger owns everything around it: the interval, the
goroutine, and the restart-on-reconnect lifecycle.
*/
type Pinger struct {
	// send writes one keepalive. It is the only part that differs per venue.
	send func() error

	// name prefixes this pinger's log lines with the session it serves.
	name string

	// mu guards stop so a reconnect can halt the previous loop before starting
	// its replacement.
	mu   sync.Mutex
	stop chan struct{}
}

func NewPinger(name string, send func() error) *Pinger {
	return &Pinger{name: name, send: send}
}

/*
Start starts (or replaces) the keepalive loop. It is called on every connect,
since a reconnect is a new session and the previous loop belongs to a socket
that is already gone. The loop ends when Stop is called or ctx is done.
*/
func (pinger *Pinger) Start(ctx context.Context) {
	if pinger == nil || pinger.send == nil {
		return
	}

	pinger.mu.Lock()

	if pinger.stop != nil {
		close(pinger.stop)
	}

	stop := make(chan struct{})
	pinger.stop = stop
	pinger.mu.Unlock()

	go func() {
		ticker := time.NewTicker(system.Cfg.WebSocket.PingInterval)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				if err := pinger.send(); err != nil {
					errnie.Error(errnie.Err(
						errnie.IO,
						fmt.Sprintf("%s: ping failed", pinger.name),
						err,
					))
				}
			case <-stop:
				return
			case <-ctx.Done():
				return
			}
		}
	}()
}

/*
Stop halts the keepalive loop. A session that is closing stops pinging a socket
it is about to disconnect.
*/
func (pinger *Pinger) Stop() {
	if pinger == nil {
		return
	}

	pinger.mu.Lock()
	defer pinger.mu.Unlock()

	if pinger.stop != nil {
		close(pinger.stop)
		pinger.stop = nil
	}
}
