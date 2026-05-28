package market

import (
	"fmt"

	"github.com/qntfy/jsonparser"
	"github.com/theapemachine/symm/kraken/core"
)

/*
Channel is one Kraken websocket channel name.
*/
type Channel string

/*
IsTrade reports trade channel aliases on the websocket feed.
*/
func (channel Channel) IsTrade() bool {
	return channel == core.ChannelTrades || channel == "trade"
}

/*
IsBook reports the order-book channel.
*/
func (channel Channel) IsBook() bool {
	return channel == core.ChannelBook
}

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
