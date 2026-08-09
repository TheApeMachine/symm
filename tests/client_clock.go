package tests

import (
	"time"

	"github.com/theapemachine/errnie"
)

/*
ConfigureTime installs the deterministic clock used by REST and subscription
responses before production clients connect.
*/
func (conn *Conn) ConfigureTime(start time.Time) error {
	if start.IsZero() {
		return errnie.Err(
			errnie.Validation, "tests: fixture clock requires a start time", nil,
		)
	}

	conn.mu.Lock()
	conn.clock = start.UTC()
	conn.mu.Unlock()
	conn.transport.configureTime(start.UTC())

	return nil
}

func (conn *Conn) currentTime() time.Time {
	conn.mu.Lock()
	defer conn.mu.Unlock()

	return conn.clock
}
