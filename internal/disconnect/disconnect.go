package disconnect

import (
	"errors"
	"net"
	"strings"

	"github.com/gorilla/websocket"
)

/*
Expected reports read/write failures that mean the peer hung up or the
socket was replaced — not an application bug.
*/
func Expected(err error) bool {
	if err == nil {
		return false
	}

	if errors.Is(err, net.ErrClosed) {
		return true
	}

	var opErr *net.OpError
	if errors.As(err, &opErr) {
		return true
	}

	if websocket.IsCloseError(
		err,
		websocket.CloseNormalClosure,
		websocket.CloseGoingAway,
		websocket.CloseAbnormalClosure,
		websocket.CloseNoStatusReceived,
	) {
		return true
	}

	msg := strings.ToLower(err.Error())

	return strings.Contains(msg, "broken pipe") ||
		strings.Contains(msg, "connection reset") ||
		strings.Contains(msg, "use of closed network connection") ||
		strings.Contains(msg, "unexpected eof")
}
