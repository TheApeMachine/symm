package broker

import (
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/network"
)

/*
OpenWS dials a WebsocketClient. Paper/virtual clients are a no-op.
*/
func OpenWS(client network.Client, url string) error {
	ws, ok := client.(*network.WebsocketClient)

	if !ok || ws == nil {
		return nil
	}

	if err := ws.Open(url); err != nil {
		return errnie.Error(err)
	}

	return nil
}
