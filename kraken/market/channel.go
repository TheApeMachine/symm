package market

import (
	"fmt"

	"github.com/qntfy/jsonparser"
	"github.com/theapemachine/symm/kraken/core"
)

/*
ChannelName reads the websocket channel field without unmarshaling the full payload.
The returned string is copied out of the read buffer immediately.
*/
func ChannelName(payload []byte) (string, error) {
	channel, err := jsonparser.GetUnsafeString(payload, "channel")
	if err != nil {
		return "", fmt.Errorf("read channel: %w", err)
	}

	return string(channel), nil
}

func isTradeChannel(channel string) bool {
	return channel == core.ChannelTrades || channel == "trade"
}

func isBookChannel(channel string) bool {
	return channel == core.ChannelBook
}
