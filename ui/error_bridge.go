package ui

import (
	"strings"
	"sync"
	"time"

	"github.com/bytedance/sonic"
	"github.com/phuslu/log"
	"github.com/theapemachine/symm/nomagique/transport"
	"github.com/theapemachine/symm/telemetry"
	wire "github.com/theapemachine/symm/telemetry/generated/telemetry"
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
	ui    *transport.MapReduce[[]byte]
	ready func() bool
	// onError receives each distinct error as (source, message, caller) so the
	// diagnostics WebRTC frame can surface subsystem-attributed errors. Nil
	// keeps the bridge limited to the websocket overlay.
	onError func(source string, message string, caller string)
	mu      sync.Mutex
	lastKey string
	lastAt  time.Time
}

/*
NewErrorBridge constructs a phuslu Writer that publishes compact error frames
onto the thesis UI transport. Nil hub or thesis yields a no-op writer. ready may
be nil (always open) or gate publishes until the boot stage allows them.
onError, when non-nil, is invoked once per distinct error with a
subsystem-attributed source.
*/
func NewErrorBridge(
	hub *Hub,
	ready func() bool,
	onError func(source string, message string, caller string),
) log.Writer {
	var ui *transport.MapReduce[[]byte]

	if hub != nil && hub.thesis != nil {
		ui = hub.thesis.UI()
	}

	if ui == nil {
		return log.WriterFunc(func(*log.Entry) (int, error) {
			return 0, nil
		})
	}

	return log.IOWriter{Writer: &ErrorBridge{
		ui:      ui,
		ready:   ready,
		onError: onError,
	}}
}

/*
Write receives one JSON log line from phuslu IOWriter and, when the level is
error or higher, non-blocking-publishes it as an error frame for the overlay.
*/
func (bridge *ErrorBridge) Write(payload []byte) (int, error) {
	if bridge == nil || bridge.ui == nil || len(payload) == 0 {
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

	if bridge.onError != nil {
		bridge.onError(
			attributedErrorSource(fields),
			errorMessage(fields),
			callerField(fields),
		)
	}

	safe := safeErrorFields(fields)
	bridge.ui.Push(telemetry.Encode(&wire.FrameT{
		Type: wire.FrameErrorFrame,
		Value: &wire.ErrorFrameT{
			Level: stringMapField(safe, "level"), Source: attributedErrorSource(fields),
			Error: stringMapField(safe, "error"), Message: stringMapField(safe, "message"),
			Msg: stringMapField(safe, "msg"), Caller: stringMapField(safe, "caller"),
			Time: stringMapField(safe, "time"),
		},
	}))
	bridge.commit(fingerprint)

	return len(payload), nil
}

func stringMapField(fields map[string]any, name string) string {
	value, _ := fields[name].(string)
	return value
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

/*
callerField returns the raw phuslu caller string, if present.
*/
func callerField(fields map[string]any) string {
	caller, _ := fields["caller"].(string)

	return strings.TrimSpace(caller)
}

/*
attributedErrorSource maps a phuslu caller package to the coarsest diagnostics
section the wiring diagram draws. Individual nodes share packages, so attribution
is honest at the section level rather than guessing a specific solver.
*/
func attributedErrorSource(fields map[string]any) string {
	caller := callerField(fields)

	for _, packageName := range []string{"/broker/", "/signal/", "/logic/", "/strategy/", "/trader/"} {
		if strings.Contains(caller, packageName) {
			return strings.Trim(packageName, "/")
		}
	}

	return "system"
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
