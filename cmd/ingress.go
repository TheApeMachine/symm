package cmd

import (
	"fmt"

	"github.com/bytedance/sonic"
	"github.com/theapemachine/errnie"
)

/*
subscribeAck is the venue's acknowledgement of a subscribe request.
*/
type subscribeAck struct {
	Method  string `json:"method"`
	Success *bool  `json:"success"`
	Error   string `json:"error"`
	Result  struct {
		Channel string `json:"channel"`
		Symbol  string `json:"symbol"`
	} `json:"result"`
}

/*
subscribeRejection returns an error when buf is a subscribe acknowledgement
the venue rejected (success=false), and nil for every other frame. Order and
other method acks are not subscriptions and are left to their own handlers.
*/
func subscribeRejection(buf []byte) error {
	var ack subscribeAck

	if err := sonic.Unmarshal(buf, &ack); err != nil {
		return nil
	}

	if ack.Method != "subscribe" || ack.Success == nil || *ack.Success {
		return nil
	}

	return errnie.Err(
		errnie.NotAcceptable,
		fmt.Sprintf(
			"[ingress] subscribe rejected (channel=%q symbol=%q): %s",
			ack.Result.Channel, ack.Result.Symbol, ack.Error,
		),
		nil,
	)
}
