package tests

import (
	"time"

	"github.com/theapemachine/errnie"
)

func (conn *Conn) reconnect() {
	conn.mu.Lock()
	accepted := conn.accepted
	generation := conn.connectionGeneration
	conn.mu.Unlock()

	if accepted == nil {
		return
	}

	errnie.Error(accepted.Close())
	timer := time.NewTimer(fixtureDeliveryTimeout)
	defer timer.Stop()
	ticker := time.NewTicker(fixtureReconnectWait)
	defer ticker.Stop()

	for {
		select {
		case <-conn.ctx.Done():
			return
		case <-timer.C:
			errnie.Error(errnie.Err(
				errnie.IO, "tests: fixture websocket failed to reconnect", nil,
			))
			return
		case <-ticker.C:
			conn.mu.Lock()
			reconnected := conn.connectionGeneration > generation
			conn.mu.Unlock()

			if reconnected {
				return
			}
		}
	}
}
