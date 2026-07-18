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

var errorAllowlist = []string{"level", "error", "message", "msg", "caller", "time"}

/*
ErrorBridge forwards error-level errnie/phuslu log lines onto the UI websocket
without blocking the trading hot path. Identical floods within a short window
are dropped so chronology spam does not thrash the overlay. Frames are held
until ready() reports the boot gate has passed (Warmup), so preflight ticker
gaps never open the overlay.
*/
type ErrorBridge struct {
	messages chan<- []byte
	ready    func() bool
	mu       sync.Mutex
	lastKey  string
	lastAt   time.Time
}

/*
NewErrorBridge constructs a phuslu Writer that publishes compact error frames
on hub.Messages. Nil hub or Messages yields a no-op writer. ready may be nil
(always open) or gate publishes until the boot stage allows them.
*/
func NewErrorBridge(hub *Hub, ready func() bool) log.Writer {
	if hub == nil || hub.Messages == nil {
		return log.WriterFunc(func(*log.Entry) (int, error) {
			return 0, nil
		})
	}

	return log.IOWriter{Writer: &ErrorBridge{
		messages: hub.Messages,
		ready:    ready,
	}}
}

/*
Write receives one JSON log line from phuslu IOWriter and, when the level is
error or higher, non-blocking-publishes it as an error frame for the overlay.
*/
func (bridge *ErrorBridge) Write(payload []byte) (int, error) {
	if bridge == nil || bridge.messages == nil || len(payload) == 0 {
		return len(payload), nil
	}

	if bridge.ready != nil && !bridge.ready() {
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

	fingerprint := errorFingerprint(fields)

	if bridge.duplicate(fingerprint) {
		return len(payload), nil
	}

	frame := datura.Map[any]{"error": safeErrorFields(fields)}.Marshal()

	select {
	case bridge.messages <- frame:
		bridge.commit(fingerprint)
	default:
		// Best-effort drop when the hub is saturated; the line still lands in
		// the on-disk log and can be retried on the next distinct fingerprint.
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

func safeErrorFields(fields map[string]any) map[string]any {
	safe := make(map[string]any, len(errorAllowlist))

	for _, key := range errorAllowlist {
		if value, ok := fields[key]; ok {
			safe[key] = value
		}
	}

	return safe
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

func errorFingerprint(fields map[string]any) string {
	level, _ := fields["level"].(string)
	caller, _ := fields["caller"].(string)

	return strings.Join([]string{
		strings.ToLower(strings.TrimSpace(level)),
		strings.TrimSpace(caller),
		errorMessage(fields),
	}, "|")
}

func (bridge *ErrorBridge) duplicate(fingerprint string) bool {
	if fingerprint == "" || fingerprint == "||" {
		return false
	}

	bridge.mu.Lock()
	defer bridge.mu.Unlock()

	now := time.Now()

	if fingerprint == bridge.lastKey && now.Sub(bridge.lastAt) < errorDebounce {
		return true
	}

	return false
}

func (bridge *ErrorBridge) commit(fingerprint string) {
	if fingerprint == "" || fingerprint == "||" {
		return
	}

	bridge.mu.Lock()
	defer bridge.mu.Unlock()

	bridge.lastKey = fingerprint
	bridge.lastAt = time.Now()
}
