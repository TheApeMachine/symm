package ui

import (
	"net/http"
	"sync"

	"github.com/bytedance/sonic"
	"github.com/gorilla/websocket"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/types"
)

/*
WebSocketServer is a general WebSocket server closure node.
It listens for frontend connections and broadcasts outbound payloads to connected clients.
No hub application hack, pure Value closure.
*/
type WebSocketServer types.Value[any, any]

func NewWebSocketServer(addr, path types.String) WebSocketServer {
	var upgrader = websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool { return true },
	}
	var clients sync.Map
	var once sync.Once

	return func(in any) any {
		once.Do(func() {
			a := ":8765"
			if addr != nil {
				if evaluated := addr(in); evaluated != "" {
					a = evaluated
				}
			}
			p := "/ws"
			if path != nil {
				if evaluated := path(in); evaluated != "" {
					p = evaluated
				}
			}

			mux := http.NewServeMux()
			mux.HandleFunc(p, func(w http.ResponseWriter, r *http.Request) {
				conn, err := upgrader.Upgrade(w, r, nil)
				if err != nil {
					return
				}
				clients.Store(conn, struct{}{})
				go func() {
					defer func() {
						clients.Delete(conn)
						_ = conn.Close()
					}()
					for {
						if _, _, err := conn.ReadMessage(); err != nil {
							break
						}
					}
				}()
			})

			server := &http.Server{Addr: a, Handler: mux}
			go func() {
				if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
					errnie.Error(errnie.Err(errnie.IO, "[ui] websocket server failed", err))
				}
			}()
		})

		if in != nil {
			var payload []byte
			switch v := in.(type) {
			case []byte:
				payload = v
			case string:
				payload = []byte(v)
			default:
				data, err := sonic.Marshal(v)
				if err == nil {
					payload = data
				}
			}
			if len(payload) > 0 {
				clients.Range(func(key, value any) bool {
					if conn, ok := key.(*websocket.Conn); ok {
						_ = conn.WriteMessage(websocket.TextMessage, payload)
					}
					return true
				})
			}
		}

		return in
	}
}
