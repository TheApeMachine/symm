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

	// failed is invoked when a keepalive write fails, which is the only signal
	// a half-open socket produces. The session decides what to do about it.
	failed func(err error)

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
OnFailed installs the handler invoked when a keepalive write fails.

A failed ping is the only evidence a half-open socket produces. The venue's read
loop is still blocked on a socket whose write side is already gone, so it never
errors, never reports the disconnect, and never triggers the reconnect that
would restore the session. Without this the loop writes into a dead socket for
the rest of the process lifetime.
*/
func (pinger *Pinger) OnFailed(handler func(err error)) {
	if pinger == nil {
		return
	}

	pinger.failed = handler
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
				err := pinger.send()

				if err == nil {
					continue
				}

				errnie.Error(errnie.Err(
					errnie.IO,
					fmt.Sprintf("%s: ping failed", pinger.name),
					err,
				))

				// The socket this loop was keeping alive is gone. Reporting it
				// once and standing down is what lets the session tear the
				// connection down and reconnect; continuing to tick would keep
				// writing into a dead socket and report the same failure
				// forever.
				if pinger.failed != nil {
					pinger.failed(err)
				}

				return
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
