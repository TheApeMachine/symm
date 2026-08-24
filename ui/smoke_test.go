package ui

import (
	"context"
	"net"
	"testing"
	"time"

	gorillawebsocket "github.com/gorilla/websocket"
	"github.com/theapemachine/symm/nomagique/runtime"
	"github.com/theapemachine/symm/types"
	wire "github.com/theapemachine/symm/telemetry/generated/telemetry"
)

/*
TestHubWebsocketBusSmoke boots a Hub against the system bus and connects one
real websocket client through /ws, then verifies a published UI frame reaches
the client. It is the regression guard for the nil-bus panic on the dashboard
socket path.
*/
func TestHubWebsocketBusSmoke(t *testing.T) {
	bus := runtime.NewWorkspace(nil)
	defer bus.Close()

	hub := NewHub(context.Background(), types.NewThesis(context.Background()), nil, nil, nil, bus)
	hub.SetDiagnosticsControl(nil)

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}

	addr := listener.Addr().String()
	listener.Close()

	go func() {
		_ = hub.app.Listen(addr)
	}()
	defer hub.app.Shutdown()

	deadline := time.Now().Add(2 * time.Second)

	for time.Now().Before(deadline) {
		conn, _, err := gorillawebsocket.DefaultDialer.Dial("ws://"+addr+"/ws", nil)
		if err != nil {
			time.Sleep(20 * time.Millisecond)
			continue
		}

		ui := runtime.ChannelOf[*types.UIFrame](bus, types.ChannelUI,
			func(frame *types.UIFrame) string { return "" })

		conn.SetReadDeadline(time.Now().Add(3 * time.Second))

		go func() {
			ticker := time.NewTicker(10 * time.Millisecond)
			defer ticker.Stop()

			for range ticker.C {
				ui.Publish(&types.UIFrame{
					Type: wire.FrameTickFrame,
					Value: &wire.TickFrameT{
						Count: 1,
					},
				})
			}
		}()

		_, payload, err := conn.ReadMessage()

		conn.Close()

		if err != nil {
			t.Fatalf("dashboard socket failed after bus publish: %v", err)
		}

		if len(payload) == 0 {
			t.Fatal("dashboard socket returned an empty frame")
		}

		return
	}

	t.Fatal("dashboard websocket did not come up")
}
