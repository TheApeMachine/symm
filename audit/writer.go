package audit

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/bytedance/sonic"
	"github.com/spf13/viper"
)

const (
	defaultMaxMegabytes = 32
	defaultBackupCount  = 3
)

/*
Writer appends desk audit frames as JSONL when trading.audit.file is configured.
*/
type Writer struct {
	path        string
	file        *os.File
	writer      *bufio.Writer
	seq         atomic.Int64
	mu          sync.Mutex
	maxBytes    int64
	backupCount int
}

/*
OpenWriter installs the process-wide audit writer when a path is configured.
*/
func OpenWriter() (*Writer, error) {
	path := auditPath()

	if path == "" {
		return nil, nil
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("audit: mkdir %q: %w", filepath.Dir(path), err)
	}

	file, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)

	if err != nil {
		return nil, fmt.Errorf("audit: open %q: %w", path, err)
	}

	maxMegabytes := viper.GetInt64("trading.audit.max_mb")

	if maxMegabytes <= 0 {
		maxMegabytes = defaultMaxMegabytes
	}

	backupCount := viper.GetInt("trading.audit.backup_count")

	if backupCount <= 0 {
		backupCount = defaultBackupCount
	}

	writer := &Writer{
		path:        path,
		file:        file,
		writer:      bufio.NewWriter(file),
		maxBytes:    maxMegabytes * 1024 * 1024,
		backupCount: backupCount,
	}

	return writer, nil
}

/*
Write appends one audit frame as a JSONL line.
*/
func (writer *Writer) Write(frame map[string]any) error {
	if writer == nil {
		return nil
	}

	writer.mu.Lock()
	defer writer.mu.Unlock()

	frame["event"] = "audit"
	frame["ts"] = time.Now().UTC().Format(time.RFC3339Nano)
	frame["seq"] = writer.seq.Add(1)

	raw, err := sonic.Marshal(frame)

	if err != nil {
		return fmt.Errorf("audit: marshal: %w", err)
	}

	if _, err := writer.writer.Write(append(raw, '\n')); err != nil {
		return fmt.Errorf("audit: write: %w", err)
	}

	if err := writer.writer.Flush(); err != nil {
		return fmt.Errorf("audit: flush: %w", err)
	}

	return writer.rotateIfNeeded()
}

/*
Close flushes and closes the audit writer.
*/
func (writer *Writer) Close() error {
	if writer == nil {
		return nil
	}

	writer.mu.Lock()
	defer writer.mu.Unlock()

	var closeErr error

	if writer.writer != nil {
		closeErr = writer.writer.Flush()
		writer.writer = nil
	}

	if writer.file != nil {
		if err := writer.file.Close(); err != nil && closeErr == nil {
			closeErr = err
		}

		writer.file = nil
	}

	return closeErr
}

func auditPath() string {
	path := strings.TrimSpace(viper.GetString("trading.audit.file"))

	if path != "" {
		return path
	}

	return strings.TrimSpace(os.Getenv("SYMM_AUDIT_FILE"))
}

func (writer *Writer) rotateIfNeeded() error {
	info, err := writer.file.Stat()

	if err != nil {
		return fmt.Errorf("audit: stat: %w", err)
	}

	if info.Size() < writer.maxBytes {
		return nil
	}

	if err := writer.writer.Flush(); err != nil {
		return fmt.Errorf("audit: rotate flush: %w", err)
	}

	if err := writer.file.Close(); err != nil {
		return fmt.Errorf("audit: rotate close: %w", err)
	}

	for index := writer.backupCount; index >= 1; index-- {
		from := rotatedPath(writer.path, index-1)
		to := rotatedPath(writer.path, index)

		if index == 1 {
			from = writer.path
		}

		if _, statErr := os.Stat(from); statErr != nil {
			continue
		}

		if err := os.Rename(from, to); err != nil {
			return fmt.Errorf("audit: rotate rename %q: %w", from, err)
		}
	}

	file, err := os.OpenFile(writer.path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)

	if err != nil {
		return fmt.Errorf("audit: rotate open: %w", err)
	}

	writer.file = file
	writer.writer = bufio.NewWriter(file)

	return nil
}

func rotatedPath(path string, index int) string {
	return fmt.Sprintf("%s.%d", path, index)
}
