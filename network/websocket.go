package network

import (
	"context"

	gorilla "github.com/gorilla/websocket"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/runtime"
)

type WebsocketClient struct {
	*runtime.System
	conn   *gorilla.Conn
	pinger *Pinger
}

func NewWebsocketClient(ctx context.Context) *WebsocketClient {
	client := &WebsocketClient{}
	client.System = runtime.NewSystem(ctx, "network.websocket", client)
	client.pinger = NewPinger(ctx, client)
	return client
}

func (client *WebsocketClient) Open(url string) error {
	var err error
	client.conn, _, err = gorilla.DefaultDialer.Dial(url, nil)

	if err != nil {
		return client.Error(errnie.Err(
			errnie.IO,
			"[network.websocket] error dialing url",
			err,
		))
	}

	client.Transition(runtime.READY)
	client.pinger.Start()
	return nil
}

func (client *WebsocketClient) Read() ([]byte, error) {
	if client.Status() != runtime.READY {
		return nil, client.Error(errnie.Err(
			errnie.IO,
			"[network.websocket] client is not ready",
			nil,
		))
	}

	_, message, err := client.conn.ReadMessage()

	if err != nil {
		return nil, client.Error(errnie.Err(
			errnie.IO,
			"[network.websocket] error reading message",
			err,
		))
	}

	return message, nil
}

func (client *WebsocketClient) Write(message []byte) error {
	if client.Status() != runtime.READY {
		return client.Error(errnie.Err(
			errnie.IO,
			"[network.websocket] client is not ready",
			nil,
		))
	}

	err := client.conn.WriteMessage(gorilla.TextMessage, message)

	if err != nil {
		return client.Error(errnie.Err(
			errnie.IO,
			"[network.websocket] error writing message",
			err,
		))
	}

	return nil
}
