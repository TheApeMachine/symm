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
	path, enabled := replayCaptureConfig()
	if !enabled {
		return nil
	}

	return &replayCapture{path: path}
}

func replayCaptureConfig() (string, bool) {
	path := strings.TrimSpace(viper.GetString("optimizer.replay.file"))
	if path == "" {
		path = "runs/replay.jsonl"
	}

	enabled := false
	if viper.IsSet("optimizer.replay.capture") {
		enabled = viper.GetBool("optimizer.replay.capture")
	}
	env := strings.TrimSpace(os.Getenv("SYMM_REPLAY_CAPTURE"))
	if env != "" {
		switch strings.ToLower(env) {
		case "0", "false", "off", "no":
			enabled = false
		case "1", "true", "on", "yes":
			enabled = true
		default:
			enabled = true
			path = env
		}
	}

	return path, enabled
}

func (capture *replayCapture) Write(artifact *datura.Artifact) {
	if capture == nil || artifact == nil {
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

	row := map[string]any{
		"origin":    origin,
		"role":      role,
		"scope":     scope,
		"type":      datura.Peek[string](artifact, "type"),
		"timestamp": artifact.Timestamp(),
		"payload":   payload,
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

	errnie.Error(json.NewEncoder(capture.file).Encode(row))
}

func replayCaptureRole(role string) bool {
	switch role {
	case "ticker", "trade", "book", "level3", "ohlc":
		return true
	default:
		return false
	}
}
