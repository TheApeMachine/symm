package internal

import (
	"context"
	"errors"
	"io"
	"net"
	"strings"
	"syscall"

	"github.com/theapemachine/errnie"
)

/*
IsShutdown reports expected termination from context cancellation.
*/
func IsShutdown(err error) bool {
	return errors.Is(err, context.Canceled)
}

/*
IsClientDisconnect reports websocket clients that closed before a write finished.
*/
func IsClientDisconnect(err error) bool {
	if err == nil {
		return false
	}

	if errors.Is(err, io.EOF) ||
		errors.Is(err, syscall.EPIPE) ||
		errors.Is(err, syscall.ECONNRESET) {
		return true
	}

	var networkError *net.OpError

	if errors.As(err, &networkError) && networkError.Err != nil {
		if errors.Is(networkError.Err, syscall.EPIPE) ||
			errors.Is(networkError.Err, syscall.ECONNRESET) {
			return true
		}
	}

	message := strings.ToLower(err.Error())

	return strings.Contains(message, "broken pipe") ||
		strings.Contains(message, "connection reset by peer") ||
		strings.Contains(message, "use of closed network connection")
}

/*
ReportError logs unexpected errors and passes shutdown errors through silently.
*/
func ReportError(err error) error {
	if err == nil || IsShutdown(err) || IsClientDisconnect(err) {
		return err
	}

	return errnie.Error(err)
}
