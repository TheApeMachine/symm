package ui

import (
	"github.com/fasthttp/websocket"
	"github.com/theapemachine/errnie"
)

type outboundEvent struct {
	value any
}

func (hub *Hub) publishToClients(value any) {
	hub.sessions.Range(func(key, stored any) bool {
		connID, ok := key.(uint64)

		if !ok {
			hub.sessions.Delete(key)
			return true
		}

		session, ok := stored.(*clientSession)

		if !ok {
			hub.detachClient(connID)
			return true
		}

		if err := session.publish(value); err != nil {
			errnie.Error(err)
		}

		return true
	})
}

func (hub *Hub) writeClient(connID uint64, value any) error {
	raw, ok := hub.clients.Load(connID)

	if !ok {
		return nil
	}

	client, ok := raw.(*websocket.Conn)

	if !ok {
		hub.detachClient(connID)
		return nil
	}

	if err := client.WriteJSON(value); err != nil {
		hub.detachClient(connID)
		return err
	}

	return nil
}
