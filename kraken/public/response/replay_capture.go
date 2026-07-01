package response

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/bytedance/sonic"
	"github.com/spf13/viper"
	"github.com/theapemachine/datura"
	"github.com/theapemachine/errnie"
)

type replayCapture struct {
	mu   sync.Mutex
	file *os.File
	path string
}

func newReplayCapture() *replayCapture {
	path := strings.TrimSpace(viper.GetString("optimizer.replay.file"))
	if path == "" {
		path = "runs/replay.jsonl"
	}

	if !viper.GetBool("optimizer.replay.capture") {
		return nil
	}

	return &replayCapture{path: path}
}

func (capture *replayCapture) Write(artifact *datura.Artifact) {
	if capture == nil || artifact == nil || !artifact.IsValid() {
		return
	}

	role := datura.Peek[string](artifact, "role")
	if !replayCaptureRole(role) {
		return
	}

	scope, _ := artifact.Scope()
	if scope == "" {
		return
	}

	var payload map[string]any
	if err := sonic.Unmarshal(artifact.DecryptPayload(), &payload); err != nil {
		return
	}

	origin, _ := artifact.Origin()
	if origin == "" {
		origin = "kraken:public"
	}

	capture.mu.Lock()
	defer capture.mu.Unlock()

	if capture.file == nil {
		if err := os.MkdirAll(filepath.Dir(capture.path), 0o755); err != nil {
			errnie.Error(err)
			return
		}

		file, err := os.OpenFile(capture.path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
		if err != nil {
			errnie.Error(err)
			return
		}
		capture.file = file
	}

	errnie.Error(json.NewEncoder(capture.file).Encode(map[string]any{
		"origin":    origin,
		"role":      role,
		"scope":     scope,
		"type":      datura.Peek[string](artifact, "type"),
		"timestamp": artifact.Timestamp(),
		"payload":   payload,
	}))
}

func replayCaptureRole(role string) bool {
	switch role {
	case "ticker", "trade", "book", "level3", "ohlc":
		return true
	default:
		return false
	}
}
