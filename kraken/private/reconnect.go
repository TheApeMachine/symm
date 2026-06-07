package private

import (
	"time"

	"github.com/bytedance/sonic"
)

func cloneOutboundFrame(value any) (any, bool) {
	raw, err := sonic.Marshal(value)

	if err != nil {
		return nil, false
	}

	var cloned any

	if err := sonic.Unmarshal(raw, &cloned); err != nil {
		return nil, false
	}

	return cloned, true
}

func reconnectDelay(attempt uint64) time.Duration {
	if attempt == 0 {
		return 0
	}

	seconds := attempt

	const maxSeconds = 30

	if seconds > maxSeconds {
		seconds = maxSeconds
	}

	return time.Duration(seconds) * time.Second
}
