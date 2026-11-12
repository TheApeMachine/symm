package network

import (
	"strings"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/internal/disconnect"
)

/*
Recoverable reports errors that ingress loops should retry without ERROR spam:
transient disconnects, redial backoff, and reconnect hooks still running.
*/
func Recoverable(err error) bool {
	if err == nil {
		return false
	}

	if disconnect.Expected(err) {
		return true
	}

	msg := strings.ToLower(err.Error())

	if strings.Contains(msg, "reconnect resubscribe in progress") ||
		strings.Contains(msg, "not ready for ping") ||
		strings.Contains(msg, "reconnect hook failed") ||
		strings.Contains(msg, "error dialing url") ||
		strings.Contains(msg, "connection missing") {
		return true
	}

	if wrapped, ok := errnie.AsErrnie(err); ok && wrapped.Unwrap() != nil {
		return Recoverable(wrapped.Unwrap())
	}

	return false
}
