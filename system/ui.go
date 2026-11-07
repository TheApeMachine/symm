package system

import (
	"github.com/spf13/viper"
)

type UI struct {
	Addr      string
	WebSocket *UIWebSocket
}

type UIWebSocket struct {
	MaxMessageBytes int
	QueueFrames     int
}

func NewUI() *UI {
	viper.SetDefault("ui.addr", "127.0.0.1:8765")
	viper.SetDefault("ui.websocket.max_message_bytes", 4194304)
	viper.SetDefault("ui.websocket.queue_frames", 256)

	return &UI{
		Addr: viper.GetString("ui.addr"),
		WebSocket: &UIWebSocket{
			MaxMessageBytes: viper.GetInt("ui.websocket.max_message_bytes"),
			QueueFrames:     viper.GetInt("ui.websocket.queue_frames"),
		},
	}
}
