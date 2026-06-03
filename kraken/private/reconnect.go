package private

import (
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
