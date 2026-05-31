package trader

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

const (
	defaultAuditMaxBytes           = 32 << 20
	defaultAuditMaxFiles           = 3
	defaultAuditGateRejectCooldown = 60 * time.Second
)

/*
AuditLog appends desk audit frames as JSONL with size-based rotation.
Gate rejects are deduplicated per symbol/playbook/reason so a steady deny
state cannot grow the file without bound.
*/
type AuditLog struct {
	path         string
	maxBytes     int64
	maxFiles     int
	gateCooldown time.Duration

	mu           sync.Mutex
	file         *os.File
	bytesWritten int64
	seq          atomic.Uint64
	gateLast     map[string]time.Time
}

/*
OpenAuditLog creates or truncates path and prepares rotation limits.
*/
func OpenAuditLog(
	path string,
	maxBytes int64,
	maxFiles int,
	gateCooldown time.Duration,
) (*AuditLog, error) {
	if strings.TrimSpace(path) == "" {
		return nil, fmt.Errorf("audit log path is required")
	}

	if maxBytes <= 0 {
		maxBytes = defaultAuditMaxBytes
	}

	if maxFiles <= 0 {
		maxFiles = defaultAuditMaxFiles
	}

	if gateCooldown <= 0 {
		gateCooldown = defaultAuditGateRejectCooldown
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}

	file, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)

	if err != nil {
		return nil, err
	}

	return &AuditLog{
		path:         path,
		maxBytes:     maxBytes,
		maxFiles:     maxFiles,
		gateCooldown: gateCooldown,
		file:         file,
		gateLast:     make(map[string]time.Time),
	}, nil
}

/*
Append writes one audit record. Returns nil when a deduped gate_reject is skipped.
*/
func (auditLog *AuditLog) Append(event string, frame map[string]any) error {
	if auditLog == nil || auditLog.file == nil {
		return nil
	}

	now := time.Now()

	auditLog.mu.Lock()
	defer auditLog.mu.Unlock()

	if event == "gate_reject" {
		key := gateRejectKey(frame)

		if auditLog.shouldSkipGateReject(key, now) {
			return nil
		}

		auditLog.gateLast[key] = now
	}

	if err := auditLog.rotateIfNeededLocked(); err != nil {
		return err
	}

	record := map[string]any{
		"event":       "audit",
		"audit_event": event,
		"seq":         auditLog.seq.Add(1),
		"ts":          now.UTC().Format(time.RFC3339Nano),
	}

	for key, value := range frame {
		record[key] = value
	}

	encoded, err := json.Marshal(record)

	if err != nil {
		return err
	}

	encoded = append(encoded, '\n')

	written, err := auditLog.file.Write(encoded)

	if err != nil {
		return err
	}

	auditLog.bytesWritten += int64(written)

	return nil
}

/*
Close flushes and releases the log file.
*/
func (auditLog *AuditLog) Close() error {
	if auditLog == nil || auditLog.file == nil {
		return nil
	}

	auditLog.mu.Lock()
	defer auditLog.mu.Unlock()

	file := auditLog.file
	auditLog.file = nil

	return file.Close()
}

func (auditLog *AuditLog) shouldSkipGateReject(key string, now time.Time) bool {
	last, seen := auditLog.gateLast[key]

	if !seen {
		return false
	}

	return now.Sub(last) < auditLog.gateCooldown
}

func gateRejectKey(frame map[string]any) string {
	symbol, _ := frame["symbol"].(string)
	playbook, _ := frame["playbook"].(string)
	reason, _ := frame["reason"].(string)

	return symbol + "|" + playbook + "|" + reason
}

func (auditLog *AuditLog) rotateIfNeededLocked() error {
	if auditLog.bytesWritten < auditLog.maxBytes {
		return nil
	}

	if err := auditLog.file.Close(); err != nil {
		return err
	}

	auditLog.file = nil
	auditLog.bytesWritten = 0

	if err := rotateAuditFiles(auditLog.path, auditLog.maxFiles); err != nil {
		return err
	}

	file, err := os.OpenFile(auditLog.path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)

	if err != nil {
		return err
	}

	auditLog.file = file

	return nil
}

func rotateAuditFiles(path string, maxFiles int) error {
	oldest := path + fmt.Sprintf(".%d", maxFiles-1)

	if err := os.Remove(oldest); err != nil && !os.IsNotExist(err) {
		return err
	}

	for index := maxFiles - 2; index >= 1; index-- {
		source := path + fmt.Sprintf(".%d", index)
		target := path + fmt.Sprintf(".%d", index+1)

		if err := os.Rename(source, target); err != nil && !os.IsNotExist(err) {
			return err
		}
	}

	backup := path + ".1"

	if err := os.Rename(path, backup); err != nil {
		return err
	}

	return nil
}
