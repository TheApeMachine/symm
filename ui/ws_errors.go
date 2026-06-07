package ui

import (
	"errors"
	"net"
	"strings"
	"syscall"
)

func isBenignWriteError(err error) bool {
	if err == nil {
		return false
	}

	if errors.Is(err, net.ErrClosed) {
		return true
	}

	var opErr *net.OpError

	if errors.As(err, &opErr) && opErr.Err != nil {
		var errno syscall.Errno

		if errors.As(opErr.Err, &errno) {
			return errno == syscall.EPIPE || errno == syscall.ECONNRESET
		}
	}

	message := strings.ToLower(err.Error())

	return strings.Contains(message, "broken pipe") ||
		strings.Contains(message, "connection reset by peer")
}
