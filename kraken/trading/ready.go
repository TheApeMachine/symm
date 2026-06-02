package trading

import "sync/atomic"

var deskReady atomic.Bool

/*
MarkDeskReady records that the private order path has produced a balance snapshot.
Story entry actions wait on this so the first playbook match does not race paper boot.
*/
func MarkDeskReady() {
	deskReady.Store(true)
}

/*
DeskReady reports whether the trading desk has completed its startup handshake.
*/
func DeskReady() bool {
	return deskReady.Load()
}

/*
ResetDeskReady clears readiness state for tests.
*/
func ResetDeskReady() {
	deskReady.Store(false)
}
