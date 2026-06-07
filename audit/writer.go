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
	pool        *WriterPool
	seq         atomic.Int64
	maxBytes    int64
	backupCount int
	queue       *writerQueue
	done        chan struct{}
	closeOnce   sync.Once
	err         atomic.Pointer[writerFailure]
}

type writerFailure struct {
	err error
}

var defaultWriterPool = NewWriterPool()

/*
OpenWriter opens the configured audit path via the process-default writer pool.
Prefer injecting audit.WriterPool from runtime.Runtime in new code.
*/
func OpenWriter() (*Writer, error) {
	return defaultWriterPool.OpenConfigured()
}

func openWriter(path string) (*Writer, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("audit: mkdir %q: %w", filepath.Dir(path), err)
	}

	file, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)

	if err != nil {
		return nil, fmt.Errorf("audit: open %q: %w", path, err)
	}

	info, err := file.Stat()

	if err != nil {
		_ = file.Close()

		return nil, fmt.Errorf("audit: stat %q: %w", path, err)
	}

	maxBytes := viper.GetInt64("trading.audit.max_bytes")

	if maxBytes <= 0 {
		maxMegabytes := viper.GetInt64("trading.audit.max_mb")

		if maxMegabytes <= 0 {
			maxMegabytes = defaultMaxMegabytes
		}

		maxBytes = maxMegabytes * 1024 * 1024
	}

	backupCount := viper.GetInt("trading.audit.backup_count")

	if backupCount <= 0 {
		backupCount = defaultBackupCount
	}

	writer := &Writer{
		path:        path,
		maxBytes:    maxBytes,
		backupCount: backupCount,
		queue:       newWriterQueue(),
		done:        make(chan struct{}),
	}

	go writer.run(file, bufio.NewWriter(file), info.Size())

	return writer, nil
}

/*
Write appends one audit frame as a JSONL line.
*/
func (writer *Writer) Write(frame map[string]any) error {
	if writer == nil {
		return nil
	}

	if err := writer.Err(); err != nil {
		return err
	}

	next := make(map[string]any, len(frame)+3)

	for key, value := range frame {
		next[key] = value
	}

	next["event"] = "audit"
	next["ts"] = time.Now().UTC().Format(time.RFC3339Nano)
	next["seq"] = writer.seq.Add(1)

	return writer.queue.Push(next)
}

/*
Close flushes and closes the audit writer.
*/
func (writer *Writer) Close() error {
	if writer == nil {
		return nil
	}

	if writer.pool != nil {
		return writer.pool.release(writer)
	}

	return writer.closeDirect()
}

func (writer *Writer) closeDirect() error {
	writer.closeOnce.Do(func() {
		writer.queue.Close()
		<-writer.done
	})

	return writer.Err()
}

/*
Err returns the first asynchronous writer failure, if one has occurred.
*/
func (writer *Writer) Err() error {
	if writer == nil {
		return nil
	}

	failure := writer.err.Load()

	if failure == nil {
		return nil
	}

	return failure.err
}

func (writer *Writer) fail(err error) {
	if err == nil {
		return
	}

	writer.err.CompareAndSwap(nil, &writerFailure{err: err})
}

func auditPath() string {
	path := strings.TrimSpace(viper.GetString("trading.audit.file"))

	if path != "" {
		return path
	}

	return strings.TrimSpace(os.Getenv("SYMM_AUDIT_FILE"))
}

type fileWriter struct {
	path        string
	file        *os.File
	writer      *bufio.Writer
	bytes       int64
	maxBytes    int64
	backupCount int
}

func (writer *Writer) run(file *os.File, buffered *bufio.Writer, bytesWritten int64) {
	defer close(writer.done)

	fileWriter := &fileWriter{
		path:        writer.path,
		file:        file,
		writer:      buffered,
		bytes:       bytesWritten,
		maxBytes:    writer.maxBytes,
		backupCount: writer.backupCount,
	}

	for {
		frame, ok := writer.queue.Pop()

		if !ok {
			break
		}

		if err := fileWriter.Write(frame); err != nil {
			writer.fail(err)

			break
		}
	}

	writer.fail(fileWriter.Close())
}

func (fileWriter *fileWriter) Write(frame map[string]any) error {
	raw, err := sonic.Marshal(frame)

	if err != nil {
		return fmt.Errorf("audit: marshal: %w", err)
	}

	line := append(raw, '\n')

	if _, err := fileWriter.writer.Write(line); err != nil {
		return fmt.Errorf("audit: write: %w", err)
	}

	fileWriter.bytes += int64(len(line))

	return fileWriter.rotateIfNeeded()
}

func (fileWriter *fileWriter) Close() error {
	var closeErr error

	if fileWriter.writer != nil {
		closeErr = fileWriter.writer.Flush()
		fileWriter.writer = nil
	}

	if fileWriter.file != nil {
		if err := fileWriter.file.Close(); err != nil && closeErr == nil {
			closeErr = err
		}

		fileWriter.file = nil
	}

	return closeErr
}

func (fileWriter *fileWriter) rotateIfNeeded() error {
	if fileWriter.bytes < fileWriter.maxBytes {
		return nil
	}

	if err := fileWriter.writer.Flush(); err != nil {
		return fmt.Errorf("audit: rotate flush: %w", err)
	}

	if err := fileWriter.file.Close(); err != nil {
		return fmt.Errorf("audit: rotate close: %w", err)
	}

	for index := fileWriter.backupCount; index >= 1; index-- {
		from := rotatedPath(fileWriter.path, index-1)
		to := rotatedPath(fileWriter.path, index)

		if index == 1 {
			from = fileWriter.path
		}

		if _, statErr := os.Stat(from); statErr != nil {
			if os.IsNotExist(statErr) {
				continue
			}

			return fmt.Errorf("audit: rotate stat %q: %w", from, statErr)
		}

		if err := os.Rename(from, to); err != nil {
			return fmt.Errorf("audit: rotate rename %q: %w", from, err)
		}
	}

	file, err := os.OpenFile(fileWriter.path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)

	if err != nil {
		return fmt.Errorf("audit: rotate open: %w", err)
	}

	fileWriter.file = file
	fileWriter.writer = bufio.NewWriter(file)
	fileWriter.bytes = 0

	return nil
}

func rotatedPath(path string, index int) string {
	return fmt.Sprintf("%s.%d", path, index)
}
