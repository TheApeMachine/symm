package record

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

type recordJob struct {
	eventType string
	value     any
}

/*
Writer appends JSONL capture frames through a background worker.
All file state lives on that goroutine; producers only send on the queue.
*/
type Writer struct {
	ctx    context.Context
	cancel context.CancelFunc
	path   string
	queue  chan recordJob
	drops  atomic.Uint64
	done   chan struct{}
	file   *os.File
	err    error
}

func NewWriter(ctx context.Context) (*Writer, error) {
	path := viper.GetString("trading.record.file")

	if path == "" {
		return nil, nil
	}

	writerCtx, cancel := context.WithCancel(ctx)

	writer := &Writer{
		ctx:    writerCtx,
		cancel: cancel,
		path:   path,
		queue:  make(chan recordJob, 4096),
		done:   make(chan struct{}),
	}

	go writer.run()

	return writer, nil
}

func (writer *Writer) Write(eventType string, value any) error {
	if writer == nil {
		return nil
	}

	if eventType == "" {
		return errnie.Error(errors.New("record: event type required"))
	}

	select {
	case writer.queue <- recordJob{eventType: eventType, value: value}:
		return nil
	case <-writer.ctx.Done():
		return errnie.Error(writer.ctx.Err())
	}
}

func (writer *Writer) Drops() uint64 {
	if writer == nil {
		return 0
	}

	return writer.drops.Load()
}

func (writer *Writer) Tick() error {
	<-writer.ctx.Done()

	return errnie.Error(writer.ctx.Err())
}

func (writer *Writer) Close() error {
	if writer == nil {
		return nil
	}

	writer.cancel()
	<-writer.done

	return errnie.Error(writer.err)
}

func (writer *Writer) run() {
	defer close(writer.done)

	if err := writer.openFile(); err != nil {
		writer.err = err
		return
	}

	defer writer.closeFile()

	for {
		select {
		case <-writer.ctx.Done():
			writer.drain()
			return
		case job := <-writer.queue:
			if err := writer.writeJob(job); err != nil {
				writer.err = errors.Join(writer.err, err)
			}
		}
	}
}

func (writer *Writer) drain() {
	for {
		select {
		case job := <-writer.queue:
			if err := writer.writeJob(job); err != nil {
				writer.err = errors.Join(writer.err, err)
			}
		default:
			return
		}
	}
}

func (writer *Writer) writeJob(job recordJob) error {
	frame := map[string]any{
		"ts":    time.Now().UTC().Format(time.RFC3339Nano),
		"type":  job.eventType,
		"value": job.value,
	}

	payload, err := json.Marshal(frame)

	if err != nil {
		return errnie.Error(err)
	}

	payload = append(payload, '\n')

	if writer.file == nil {
		return errnie.Error(errors.New("record: file closed"))
	}

	_, err = writer.file.Write(payload)

	return errnie.Error(err)
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

func ResolveCapturePath(flagPath string, recordEnabled bool) string {
	if envPath := viper.GetString("SYMM_RECORD_FILE"); envPath != "" {
		return envPath
	}

	if flagPath != "" {
		return flagPath
	}

	if recordEnabled {
		return "runs/capture.jsonl"
	}

	return viper.GetString("trading.record.file")
}

func BindCapturePath(path string) error {
	if path == "" {
		return nil
	}

	viper.Set("trading.record.file", path)

	return nil
}

func ValidateCapturePath(path string) error {
	if path == "" {
		return nil
	}

	if filepath.Ext(path) != ".jsonl" {
		return fmt.Errorf("record: capture path must end with .jsonl")
	}

	return nil
}
