package client

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/fasthttp/websocket"
)

type testWSServer struct {
	server *httptest.Server
	url    string
}

func newTestWSServer(t *testing.T) *testWSServer {
	t.Helper()

	upgrader := websocket.Upgrader{}
	handler := http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		conn, err := upgrader.Upgrade(writer, request, nil)
		if err != nil {
			return
		}
		defer conn.Close()

		for {
			_, payload, err := conn.ReadMessage()
			if err != nil {
				return
			}

			response := handleTestFrame(payload)
			if len(response) == 0 {
				continue
			}

			if err := conn.WriteMessage(websocket.TextMessage, response); err != nil {
				return
			}
		}
	})

	server := httptest.NewServer(handler)

	return &testWSServer{
		server: server,
		url:    strings.Replace(server.URL, "http://", "ws://", 1),
	}
}

func (testServer *testWSServer) Close() {
	testServer.server.Close()
}

func handleTestFrame(payload []byte) []byte {
	var frame struct {
		Method string         `json:"method"`
		Params map[string]any `json:"params"`
	}
	if err := json.Unmarshal(payload, &frame); err != nil {
		return nil
	}

	if frame.Method == "ping" {
		body, _ := json.Marshal(map[string]any{"method": "pong"})
		return body
	}

	if frame.Method != "subscribe" {
		return nil
	}

	channel, _ := frame.Params["channel"].(string)
	token, _ := frame.Params["token"].(string)

	body, _ := json.Marshal(map[string]any{
		"channel": channel,
		"method":  "subscribe",
		"success": true,
		"token":   token,
	})

	return body
}
