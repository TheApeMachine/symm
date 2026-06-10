package audit

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync/atomic"
	"time"

	"github.com/spf13/viper"
	"github.com/theapemachine/errnie"
)

type auditJob struct {
	dedupeKey string
	frame     map[string]any
}

/*
Writer appends JSONL audit frames through a background worker.
All mutable state lives on that goroutine; producers only send on the queue.
*/
type Writer struct {
	ctx          context.Context
	cancel       context.CancelFunc
	path         string
	queue        chan auditJob
	cooldown     time.Duration
	maxBytes     int64
	maxBackups   int
	file         *os.File
	bytesWritten int64
	drops        atomic.Uint64
	err          error
	done         chan struct{}
}

/*
NewWriter opens the configured audit log or returns nil when disabled.
*/
func NewWriter(ctx context.Context) (*Writer, error) {
	path := viper.GetString("trading.audit.file")

	if path == "" {
		return nil, nil
	}

	cooldown := viper.GetDuration("trading.audit.gate_cooldown")

	if cooldown <= 0 {
		cooldown = 60 * time.Second
	}

	maxMegabytes := viper.GetInt64("trading.audit.max_mb")

	if maxMegabytes <= 0 {
		maxMegabytes = 32
	}

	maxBackups := viper.GetInt("trading.audit.max_backups")

	if maxBackups <= 0 {
		maxBackups = 3
	}

	writerCtx, cancel := context.WithCancel(ctx)

	writer := &Writer{
		ctx:        writerCtx,
		cancel:     cancel,
		path:       path,
		queue:      make(chan auditJob, 4096),
		cooldown:   cooldown,
		maxBytes:   maxMegabytes * 1024 * 1024,
		maxBackups: maxBackups,
		done:       make(chan struct{}),
	}

	if err := writer.openFile(); err != nil {
		cancel()

		return nil, errnie.Error(err)
	}

	go writer.run()

	return writer, nil
}

/*
TryEnqueueFrame appends one typed audit frame without blocking.
*/
func (writer *Writer) TryEnqueueFrame(frame Frame) bool {
	if writer == nil {
		return true
	}

	payload := framePayload(frame)

	if payload == nil {
		errnie.Error(errors.New("audit: nil frame"))
		return false
	}

	select {
	case writer.queue <- auditJob{frame: payload}:
		return true
	default:
		writer.drops.Add(1)
		return false
	}
}

/*
TryEnqueueDedupedFrame is the non-blocking variant for deduped typed frames.
*/
func (writer *Writer) TryEnqueueDedupedFrame(frame DedupedFrame) bool {
	if writer == nil {
		return true
	}

	payload := framePayload(frame)

	if payload == nil {
		errnie.Error(errors.New("audit: nil frame"))
		return false
	}

	select {
	case writer.queue <- auditJob{dedupeKey: frame.DedupeKey(), frame: payload}:
		return true
	default:
		writer.drops.Add(1)
		return false
	}
}

/*
EnqueueFrame appends one typed audit frame.
*/
func (writer *Writer) EnqueueFrame(frame Frame) error {
	if writer == nil {
		return nil
	}

	payload := framePayload(frame)

	if payload == nil {
		return errnie.Error(errors.New("audit: nil frame"))
	}

	select {
	case writer.queue <- auditJob{frame: payload}:
		return nil
	case <-writer.ctx.Done():
		return errnie.Error(writer.ctx.Err())
	}
}

/*
EnqueueDedupedFrame suppresses duplicate typed gate lines inside the cooldown window.
*/
func (writer *Writer) EnqueueDedupedFrame(frame DedupedFrame) error {
	if writer == nil {
		return nil
	}

	payload := framePayload(frame)

	if payload == nil {
		return errnie.Error(errors.New("audit: nil frame"))
	}

	select {
	case writer.queue <- auditJob{dedupeKey: frame.DedupeKey(), frame: payload}:
		return nil
	case <-writer.ctx.Done():
		return errnie.Error(writer.ctx.Err())
	}
}

/*
TryEnqueue appends one audit frame without blocking. Returns false when full.
*/
func (writer *Writer) TryEnqueue(frame map[string]any) bool {
	if writer == nil {
		return true
	}

	if frame == nil {
		errnie.Error(errors.New("audit: nil frame"))
		return false
	}

	select {
	case writer.queue <- auditJob{frame: frame}:
		return true
	default:
		writer.drops.Add(1)
		return false
	}
}

/*
TryEnqueueDeduped is the non-blocking variant of EnqueueDeduped.
*/
func (writer *Writer) TryEnqueueDeduped(dedupeKey string, frame map[string]any) bool {
	if writer == nil {
		return true
	}

	if frame == nil {
		errnie.Error(errors.New("audit: nil frame"))
		return false
	}

	select {
	case writer.queue <- auditJob{dedupeKey: dedupeKey, frame: frame}:
		return true
	default:
		writer.drops.Add(1)
		return false
	}
}

func (writer *Writer) Drops() uint64 {
	if writer == nil {
		return 0
	}

	return writer.drops.Load()
}

/*
Enqueue appends one audit frame. Blocks until the worker accepts it.
*/
func (writer *Writer) Enqueue(frame map[string]any) error {
	if writer == nil {
		return nil
	}

	if frame == nil {
		return errnie.Error(errors.New("audit: nil frame"))
	}

	select {
	case writer.queue <- auditJob{frame: frame}:
		return nil
	case <-writer.ctx.Done():
		return errnie.Error(writer.ctx.Err())
	}
}

/*
EnqueueDeduped suppresses identical gate lines inside the cooldown window.
Dedupe state is checked on the worker goroutine only.
*/
func (writer *Writer) EnqueueDeduped(dedupeKey string, frame map[string]any) error {
	if writer == nil {
		return nil
	}

	if frame == nil {
		return errnie.Error(errors.New("audit: nil frame"))
	}

	select {
	case writer.queue <- auditJob{dedupeKey: dedupeKey, frame: frame}:
		return nil
	case <-writer.ctx.Done():
		return errnie.Error(writer.ctx.Err())
	}
}

/*
Tick waits until shutdown so the engine keeps the writer alive.
*/
func (writer *Writer) Tick() error {
	<-writer.ctx.Done()

	return errnie.Error(writer.ctx.Err())
}

/*
Close drains the queue and closes the log file.
*/
func (writer *Writer) Close() error {
	if writer == nil {
		return nil
	}

	writer.cancel()
	<-writer.done

	return errnie.Error(writer.err)
}

func (writer *Writer) run() {
	dedupe := make(map[string]time.Time)

	defer writer.closeFile()
	defer close(writer.done)

	for {
		select {
		case <-writer.ctx.Done():
			writer.drain(dedupe)

			return
		case job := <-writer.queue:
			writer.processJob(job, dedupe)
		}
	}
}

func (writer *Writer) drain(dedupe map[string]time.Time) {
	for {
		select {
		case job := <-writer.queue:
			writer.processJob(job, dedupe)
		default:
			return
		}
	}
}

func (writer *Writer) processJob(job auditJob, dedupe map[string]time.Time) {
	if job.dedupeKey != "" {
		now := time.Now()
		last, seen := dedupe[job.dedupeKey]

		if seen && now.Sub(last) < writer.cooldown {
			return
		}

		dedupe[job.dedupeKey] = now
	}

	if err := writer.writeFrame(job.frame); err != nil {
		writer.err = errors.Join(writer.err, err)
	}
}

func (writer *Writer) writeFrame(frame map[string]any) error {
	if writer.file == nil {
		return errnie.Error(errors.New("audit: file closed"))
	}

	if writer.bytesWritten >= writer.maxBytes {
		if err := writer.rotate(); err != nil {
			return errnie.Error(err)
		}
	}

	payload, err := json.Marshal(frame)

	if err != nil {
		return errnie.Error(err)
	}

	payload = append(payload, '\n')

	written, err := writer.file.Write(payload)

	if err != nil {
		return errnie.Error(err)
	}

	writer.bytesWritten += int64(written)

	return nil
}

func (writer *Writer) openFile() error {
	if err := os.MkdirAll(filepath.Dir(writer.path), 0o755); err != nil {
		return errnie.Error(err)
	}

	file, err := os.OpenFile(writer.path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)

	if err != nil {
		return errnie.Error(err)
	}

	writer.file = file

	info, statErr := file.Stat()

	if statErr != nil {
		return errnie.Error(statErr)
	}

	writer.bytesWritten = info.Size()

	return nil
}

func (writer *Writer) closeFile() {
	if writer.file == nil {
		return
	}

	if err := writer.file.Close(); err != nil {
		writer.err = errors.Join(writer.err, err)
	}

	writer.file = nil
}

func (writer *Writer) rotate() error {
	if writer.file != nil {
		if err := writer.file.Close(); err != nil {
			return errnie.Error(err)
		}
	}

	for index := writer.maxBackups; index >= 1; index-- {
		source := backupPath(writer.path, index-1)
		target := backupPath(writer.path, index)

		if index == writer.maxBackups {
			_ = os.Remove(target)
		}

		if _, err := os.Stat(source); err != nil {
			continue
		}

		if err := os.Rename(source, target); err != nil {
			return errnie.Error(err)
		}
	}

	if err := os.Rename(writer.path, backupPath(writer.path, 1)); err != nil {
		return errnie.Error(err)
	}

	writer.file = nil
	writer.bytesWritten = 0

	return writer.openFile()
}

func backupPath(path string, index int) string {
	if index == 0 {
		return path
	}

	return fmt.Sprintf("%s.%d", path, index)
}
