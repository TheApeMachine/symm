package ui

import (
	"strings"
	"sync"
	"time"

	"github.com/bytedance/sonic"
	"github.com/phuslu/log"
	"github.com/theapemachine/datura"
)

const errorDebounce = 250 * time.Millisecond

/*
ErrorBridge forwards error-level errnie/phuslu log lines onto the UI websocket
without blocking the trading hot path. Identical floods within a short window
are dropped so chronology spam does not thrash the overlay.
*/
type ErrorBridge struct {
	messages chan<- []byte
	mu       sync.Mutex
	lastMsg  string
	lastAt   time.Time
}

/*
NewErrorBridge constructs a phuslu Writer that publishes compact error frames
on hub.Messages. Nil hub or Messages yields a no-op writer.
*/
func NewErrorBridge(hub *Hub) log.Writer {
	if hub == nil || hub.Messages == nil {
		return log.WriterFunc(func(*log.Entry) (int, error) {
			return 0, nil
		})
	}

	return log.IOWriter{Writer: &ErrorBridge{messages: hub.Messages}}
}

/*
Write receives one JSON log line from phuslu IOWriter and, when the level is
error or higher, non-blocking-publishes it as an error frame for the overlay.
*/
func (bridge *ErrorBridge) Write(payload []byte) (int, error) {
	if bridge == nil || bridge.messages == nil || len(payload) == 0 {
		return len(payload), nil
	}

	var fields map[string]any

	if err := sonic.Unmarshal(payload, &fields); err != nil {
		return len(payload), nil
	}

	level, _ := fields["level"].(string)

	if !isErrorLevel(level) {
		return len(payload), nil
	}

	message := errorMessage(fields)

	if bridge.duplicate(message) {
		return len(payload), nil
	}

	frame := datura.Map[any]{"error": fields}.Marshal()

	select {
	case bridge.messages <- frame:
	default:
	}

	return len(payload), nil
}

func isErrorLevel(level string) bool {
	switch strings.ToLower(strings.TrimSpace(level)) {
	case "error", "fatal", "panic":
		return true
	default:
		return false
	}
}

func errorMessage(fields map[string]any) string {
	if message, ok := fields["error"].(string); ok && message != "" {
		return message
	}

	if message, ok := fields["message"].(string); ok && message != "" {
		return message
	}

	if message, ok := fields["msg"].(string); ok && message != "" {
		return message
	}

	return ""
}

func (bridge *ErrorBridge) duplicate(message string) bool {
	if message == "" {
		return false
	}

	bridge.mu.Lock()
	defer bridge.mu.Unlock()

	now := time.Now()

	if message == bridge.lastMsg && now.Sub(bridge.lastAt) < errorDebounce {
		return true
	}

	bridge.lastMsg = message
	bridge.lastAt = now

	return false
}
