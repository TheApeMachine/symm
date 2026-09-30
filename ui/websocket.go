package ui

import (
	"context"
	"time"

	"github.com/gofiber/contrib/v3/websocket"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/runtime"
)

type WebSocket struct {
	*runtime.System
	hub *Hub
	tee runtime.Tee
}

func NewWebSocket(ctx context.Context, hub *Hub, tee runtime.Tee) *WebSocket {
	return &WebSocket{
		System: runtime.NewSystem(ctx, "ui.websocket"),
		hub:    hub,
		tee:    tee,
	}
}

func (ws *WebSocket) Register() {
	ws.hub.app.Get("/ws", websocket.New(func(conn *websocket.Conn) {
		errnie.Info("hub: frontend websocket connected")

		defer func() {
			errnie.Info("hub: frontend websocket disconnected")
			conn.Conn.Close()
		}()

		go func() {
			for {
				select {
				case <-ws.Context().Done():
					return
				default:
				}

				_, payload, err := conn.Conn.ReadMessage()

				if err != nil {
					return
				}

				ws.hub.handleCommand(payload)
			}
		}()

		for {
			select {
			case <-ws.Context().Done():
				return
			default:
			}

			if ptr := ws.tee.Next(); ptr != nil {
				conn.Conn.WriteMessage(websocket.BinaryMessage, *(*[]byte)(ptr))
			} else {
				time.Sleep(10 * time.Millisecond)
			}
		}
	}))
}
