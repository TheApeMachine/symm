package main

import (
	"fmt"
	"net/http"

	"github.com/fasthttp/websocket"
)

func main() {
	conn, resp, err := websocket.DefaultDialer.Dial(
		"wss://ws.kraken.com/v2",
		http.Header{},
	)

	if err != nil {
		fmt.Println("dial error:", err)

		return
	}

	fmt.Println("connected status:", resp.StatusCode)
	conn.Close()
}
