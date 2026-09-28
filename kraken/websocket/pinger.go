package websocket

import (
	"context"
	"fmt"
	"sync/atomic"
	"time"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/system"
)

/*
Pinger owns one session's keepalive loop. Spot and futures send different
payloads — spot a {"method":"ping"} message, futures a protocol ping control
frame, since Kraken Futures rejects an application-level ping — so the session
supplies the send and the Pinger owns everything around it: the interval, the
goroutine, and the session lifecycle.
*/
type Pinger struct {
	// send writes one keepalive. It is the only part that differs per venue.
	send func() error

	// failed is invoked when a keepalive write fails, which is the only signal
	// a half-open socket produces. The session decides what to do about it.
	failed func(err error)

	// name prefixes this pinger's log lines with the session it serves.
	name string

	stop atomic.Pointer[chan struct{}]
}

func NewPinger(name string, send func() error) *Pinger {
	return &Pinger{name: name, send: send}
}

/*
OnFailed installs the handler invoked when a keepalive write fails.

A failed ping is the only evidence a half-open socket produces. The venue's read
loop is still blocked on a socket whose write side is already gone, so it may
never report the disconnect. The session treats this callback as terminal.
*/
func (pinger *Pinger) OnFailed(handler func(err error)) {
	if pinger == nil {
		return
	}

	pinger.failed = handler
}

/*
Start starts (or replaces) the keepalive loop. The loop ends when Stop is called
or ctx is done.
*/
func (pinger *Pinger) Start(ctx context.Context) {
	if pinger == nil || pinger.send == nil {
		return
	}

	stop := make(chan struct{})
	oldStop := pinger.stop.Swap(&stop)

	if oldStop != nil && *oldStop != nil {
		close(*oldStop)
	}

	interval := system.Cfg.WebSocket.PingInterval

	go func() {
		ticker := time.NewTicker(interval)
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

				// Report once and stand down. Continuing to tick would keep
				// writing into a dead socket and repeat the same terminal fact.
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
Close halts the keepalive loop, satisfying io.Closer.
*/
func (pinger *Pinger) Close() error {
	if pinger == nil {
		return nil
	}

	oldStop := pinger.stop.Swap(nil)

	if oldStop != nil && *oldStop != nil {
		close(*oldStop)
	}

	return nil
}

/*
Stop halts the keepalive loop. A session that is closing stops pinging a socket
it is about to disconnect.
*/
func (pinger *Pinger) Stop() {
	if err := pinger.Close(); err != nil {
		errnie.Error(err)
	}
}
