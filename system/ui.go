package system

import (
	"time"

	"github.com/spf13/viper"
)

type UI struct {
	Addr      string
	WebRTC    *UIWebRTC
	WebSocket *UIWebSocket
}

type UIWebRTC struct {
	ClientQueueFrames int
	BufferedSegments  int
}

type UIWebSocket struct {
	LearningInterval time.Duration
	MaxMessageBytes  int
}

func NewUI() *UI {
	viper.SetDefault("ui.addr", "127.0.0.1:8765")
	viper.SetDefault("ui.webrtc.client_queue_frames", 4)
	viper.SetDefault("ui.webrtc.buffered_segments", 64)
	viper.SetDefault("ui.websocket.learning_interval", 100*time.Millisecond)
	viper.SetDefault("ui.websocket.max_message_bytes", 4194304)

	return &UI{
		Addr: viper.GetString("ui.addr"),
		WebRTC: &UIWebRTC{
			ClientQueueFrames: viper.GetInt("ui.webrtc.client_queue_frames"),
			BufferedSegments:  viper.GetInt("ui.webrtc.buffered_segments"),
		},
		WebSocket: &UIWebSocket{
			LearningInterval: viper.GetDuration("ui.websocket.learning_interval"),
			MaxMessageBytes:  viper.GetInt("ui.websocket.max_message_bytes"),
		},
	}
}
