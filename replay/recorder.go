package replay

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"
)

/*
Recorder appends JSONL replay lines for Kraken WebSocket and REST traffic.
*/
type Recorder struct {
	path string
	file *os.File
	mu   sync.Mutex
}

var activeRecorder struct {
	mu sync.RWMutex
	r  *Recorder
}

/*
OpenRecorder creates or truncates path and installs the process-wide recorder.
*/
func OpenRecorder(path string) (*Recorder, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}

	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)

	if err != nil {
		return nil, err
	}

	recorder := &Recorder{path: path, file: file}

	activeRecorder.mu.Lock()
	activeRecorder.r = recorder
	activeRecorder.mu.Unlock()

	return recorder, nil
}

/*
ActiveRecorder returns the installed recorder, if any.
*/
func ActiveRecorder() *Recorder {
	activeRecorder.mu.RLock()
	defer activeRecorder.mu.RUnlock()

	return activeRecorder.r
}

/*
WriteMeta records session metadata such as the symbol universe.
*/
func WriteMeta(channel string, payload any) error {
	raw, err := json.Marshal(payload)

	if err != nil {
		return err
	}

	return writeLine(Line{
		Timestamp: time.Now().UTC(),
		Transport: TransportMeta,
		Channel:   channel,
		Payload:   raw,
	})
}

/*
WriteWS records one WebSocket frame in either direction.
*/
func WriteWS(channel, direction string, payload []byte) error {
	return writeLine(Line{
		Timestamp: time.Now().UTC(),
		Transport: TransportWS,
		Channel:   channel,
		Direction: direction,
		Payload:   json.RawMessage(payload),
	})
}

/*
WriteREST records one REST response body for endpoint channel.
*/
func WriteREST(channel string, payload []byte) error {
	return writeLine(Line{
		Timestamp: time.Now().UTC(),
		Transport: TransportREST,
		Channel:   channel,
		Direction: DirectionIn,
		Payload:   json.RawMessage(payload),
	})
}

func writeLine(line Line) error {
	activeRecorder.mu.RLock()
	recorder := activeRecorder.r
	activeRecorder.mu.RUnlock()

	if recorder == nil {
		return nil
	}

	return recorder.append(line)
}

func (recorder *Recorder) append(line Line) error {
	recorder.mu.Lock()
	defer recorder.mu.Unlock()

	if line.Timestamp.IsZero() {
		line.Timestamp = time.Now().UTC()
	}

	encoded, err := json.Marshal(line)

	if err != nil {
		return err
	}

	encoded = append(encoded, '\n')

	_, err = recorder.file.Write(encoded)

	return err
}

/*
Close flushes and clears the active recorder.
*/
func (recorder *Recorder) Close() error {
	recorder.mu.Lock()
	defer recorder.mu.Unlock()

	activeRecorder.mu.Lock()

	if activeRecorder.r == recorder {
		activeRecorder.r = nil
	}

	activeRecorder.mu.Unlock()

	return recorder.file.Close()
}
