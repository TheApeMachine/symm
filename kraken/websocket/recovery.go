package websocket

import (
	"fmt"
	"sync/atomic"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/system"
)

/*
Recovery decides when a session's reconnects have stopped being worth trying.

A rotating client IP invalidates the venue session rather than the socket: every
attempt dials, authenticates, and is dropped immediately, so the transport looks
healthy while delivering nothing. Counting reconnects that produce no traffic is
what separates that from an ordinary drop, which recovers on the first attempt.

Recovery only reports the fact. What recovery means at that point — withdrawing
subscriptions, tearing every session down, restarting the process — belongs to
the owner that holds all the sessions, not to one transport.
*/
type Recovery struct {
	// name prefixes this session's log lines.
	name string

	// dead counts consecutive reconnects that delivered no frame.
	dead atomic.Int64

	// unrecoverable is invoked once when the budget is exhausted.
	unrecoverable func(reason string)

	// rebooting ensures the handler fires exactly once per session, so a burst
	// of failing reconnects requests a single reboot rather than one each.
	rebooting atomic.Bool
}

func NewRecovery(name string) *Recovery {
	return &Recovery{name: name}
}

/*
OnUnrecoverable installs the callback invoked once when this session exhausts
its reconnect budget.
*/
func (recovery *Recovery) OnUnrecoverable(handler func(reason string)) {
	if recovery == nil {
		return
	}

	recovery.unrecoverable = handler
}

/*
Delivered records that the venue accepted this session. Real inbound traffic,
not a successful dial, is what proves it, so this is called from the receive
path rather than on connect.
*/
func (recovery *Recovery) Delivered() {
	if recovery == nil {
		return
	}

	recovery.dead.Store(0)
}

/*
Dropped records one abnormal close and escalates when a session has closed
without delivering a frame enough times in a row. Reconnecting cannot fix a
session the venue is rejecting, so at that point the owner is asked to reboot.
*/
func (recovery *Recovery) Dropped(endpoint string) {
	if recovery == nil {
		return
	}

	limit := system.Cfg.WebSocket.DeadReconnectLimit

	if recovery.dead.Add(1) < limit {
		return
	}

	recovery.escalate(fmt.Sprintf(
		"%s: %d reconnects delivered no data", endpoint, limit,
	))
}

/*
escalate reports an unrecoverable transport exactly once per session.
*/
func (recovery *Recovery) escalate(reason string) {
	if recovery.unrecoverable == nil {
		return
	}

	if !recovery.rebooting.CompareAndSwap(false, true) {
		return
	}

	errnie.Warn(fmt.Sprintf(
		"%s: transport unrecoverable (%s), escalating to full reboot",
		recovery.name, reason,
	))

	go recovery.unrecoverable(reason)
}
