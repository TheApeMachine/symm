/*
Package rawdump provides a config-gated, size-rotating JSONL sink that every
signal can use to persist its richest pre-classification state to
runs/<name>_raw.jsonl.

Each signal keeps its own bespoke record struct — rawdump only owns the file
mechanics (gating, buffering, rotation), so the dumps stay maximally detailed
while the plumbing lives in one place. A nil *Writer is a no-op, so a signal can
hold one unconditionally and let configuration decide whether anything is written.

Enable a single signal with signals.<name>.raw_dump: true, or all signals at once
with signals.raw_dump: true. Rotation is governed by signals.raw_dump_max_mb
(default 64) and signals.raw_dump_backup_count (default 3).
*/
package rawdump

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/bytedance/sonic"
	"github.com/spf13/viper"
)

const (
	defaultMaxMegabytes = 64
	defaultBackupCount   = 3
	writeBufferBytes     = 64 * 1024
)

/*
Writer appends a signal's bespoke records as JSONL, rotating the file once it
crosses the configured size. All methods are safe on a nil receiver.
*/
type Writer struct {
	mutex       sync.Mutex
	path        string
	file        *os.File
	buffered    *bufio.Writer
	bytes       int64
	maxBytes    int64
	backupCount int
	opened      bool
	failed      error
}

/*
Open returns a Writer for the named signal when raw dumping is enabled for it,
otherwise nil (a no-op the caller can hold and Write to freely). The destination
defaults to runs/<name>_raw.jsonl and can be overridden per signal with
signals.<name>.raw_dump_file.
*/
func Open(name string) *Writer {
	if !enabled(name) {
		return nil
	}

	path := strings.TrimSpace(viper.GetString("signals." + name + ".raw_dump_file"))

	if path == "" {
		path = filepath.Join(Dir(), name+"_raw.jsonl")
	}

	return &Writer{
		path:        path,
		maxBytes:    resolveMaxBytes(),
		backupCount: resolveBackupCount(),
	}
}

func enabled(name string) bool {
	// An explicit per-signal value wins over the global switch in BOTH directions:
	// signals.<name>.raw_dump: false silences a signal even when the global is on,
	// and : true enables one even when the global is off. A signal that sets nothing
	// inherits signals.raw_dump.
	key := "signals." + name + ".raw_dump"

	if viper.IsSet(key) {
		return viper.GetBool(key)
	}

	return viper.GetBool("signals.raw_dump")
}

/*
Dir is the directory raw dumps are written to and read back from. It defaults to
"runs" and is overridable with signals.raw_dump_dir, so the writer and the
diagnostics reader always resolve the same location.
*/
func Dir() string {
	if dir := strings.TrimSpace(viper.GetString("signals.raw_dump_dir")); dir != "" {
		return dir
	}

	return "runs"
}

func resolveMaxBytes() int64 {
	maxBytes := viper.GetInt64("signals.raw_dump_max_bytes")

	if maxBytes > 0 {
		return maxBytes
	}

	maxMegabytes := viper.GetInt64("signals.raw_dump_max_mb")

	if maxMegabytes <= 0 {
		maxMegabytes = defaultMaxMegabytes
	}

	return maxMegabytes * 1024 * 1024
}

func resolveBackupCount() int {
	backupCount := viper.GetInt("signals.raw_dump_backup_count")

	if backupCount <= 0 {
		backupCount = defaultBackupCount
	}

	return backupCount
}

/*
Write appends one bespoke record as a JSONL line. record is any value sonic can
marshal — typically a signal-specific struct. The first failure is latched and
returned by every subsequent call so a broken sink fails loudly rather than
silently dropping telemetry.
*/
func (writer *Writer) Write(record any) error {
	if writer == nil {
		return nil
	}

	writer.mutex.Lock()
	defer writer.mutex.Unlock()

	if writer.failed != nil {
		return writer.failed
	}

	if err := writer.ensureOpen(); err != nil {
		writer.failed = err

		return err
	}

	raw, err := sonic.Marshal(record)

	if err != nil {
		return fmt.Errorf("rawdump %s: marshal: %w", writer.path, err)
	}

	line := append(raw, '\n')

	if _, err := writer.buffered.Write(line); err != nil {
		writer.failed = err

		return fmt.Errorf("rawdump %s: write: %w", writer.path, err)
	}

	writer.bytes += int64(len(line))

	if err := writer.buffered.Flush(); err != nil {
		writer.failed = err

		return fmt.Errorf("rawdump %s: flush: %w", writer.path, err)
	}

	return writer.rotateIfNeeded()
}

/*
Path returns the destination file, or "" for a disabled (nil) writer.
*/
func (writer *Writer) Path() string {
	if writer == nil {
		return ""
	}

	return writer.path
}

/*
Close flushes and closes the underlying file. Safe on a nil receiver and idempotent.
*/
func (writer *Writer) Close() error {
	if writer == nil {
		return nil
	}

	writer.mutex.Lock()
	defer writer.mutex.Unlock()

	if !writer.opened {
		return nil
	}

	flushErr := writer.buffered.Flush()
	closeErr := writer.file.Close()

	writer.opened = false
	writer.buffered = nil
	writer.file = nil

	if flushErr != nil {
		return flushErr
	}

	return closeErr
}

func (writer *Writer) ensureOpen() error {
	if writer.opened {
		return nil
	}

	if err := os.MkdirAll(filepath.Dir(writer.path), 0o755); err != nil {
		return fmt.Errorf("rawdump %s: mkdir: %w", writer.path, err)
	}

	file, err := os.OpenFile(writer.path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)

	if err != nil {
		return fmt.Errorf("rawdump %s: open: %w", writer.path, err)
	}

	info, err := file.Stat()

	if err != nil {
		_ = file.Close()

		return fmt.Errorf("rawdump %s: stat: %w", writer.path, err)
	}

	writer.file = file
	writer.buffered = bufio.NewWriterSize(file, writeBufferBytes)
	writer.bytes = info.Size()
	writer.opened = true

	return nil
}

func (writer *Writer) rotateIfNeeded() error {
	if writer.bytes < writer.maxBytes {
		return nil
	}

	if err := writer.buffered.Flush(); err != nil {
		return fmt.Errorf("rawdump %s: rotate flush: %w", writer.path, err)
	}

	if err := writer.file.Close(); err != nil {
		return fmt.Errorf("rawdump %s: rotate close: %w", writer.path, err)
	}

	for index := writer.backupCount; index >= 1; index-- {
		from := rotatedPath(writer.path, index-1)
		to := rotatedPath(writer.path, index)

		if index == 1 {
			from = writer.path
		}

		if _, statErr := os.Stat(from); statErr != nil {
			if os.IsNotExist(statErr) {
				continue
			}

			return fmt.Errorf("rawdump %s: rotate stat %q: %w", writer.path, from, statErr)
		}

		if err := os.Rename(from, to); err != nil {
			return fmt.Errorf("rawdump %s: rotate rename %q: %w", writer.path, from, err)
		}
	}

	file, err := os.OpenFile(writer.path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)

	if err != nil {
		return fmt.Errorf("rawdump %s: rotate open: %w", writer.path, err)
	}

	writer.file = file
	writer.buffered = bufio.NewWriterSize(file, writeBufferBytes)
	writer.bytes = 0

	return nil
}

func rotatedPath(path string, index int) string {
	return fmt.Sprintf("%s.%d", path, index)
}
