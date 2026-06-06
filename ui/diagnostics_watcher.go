package ui

import (
	"log"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/bytedance/sonic"
	"github.com/fsnotify/fsnotify"
	"github.com/theapemachine/symm/analyze"
	"github.com/theapemachine/symm/rawdump"
)

const (
	diagnosticsDebounce = 2 * time.Second
)

/*
diagnosticsWatcher watches the raw dump directory and pushes debounced tail analyses
to websocket clients whenever a dump grows. It also rebroadcasts the dump inventory
when files appear, rotate, or change size.
*/
type diagnosticsWatcher struct {
	hub       *Hub
	watcher   *fsnotify.Watcher
	pendingMu sync.Mutex
	pending   map[string]*time.Timer
}

func startDiagnosticsWatcher(hub *Hub) *diagnosticsWatcher {
	if hub.server == nil {
		return nil
	}

	watcher, err := fsnotify.NewWatcher()

	if err != nil {
		log.Printf("ui diagnostics: fsnotify: %v", err)

		return nil
	}

	dir := rawdump.Dir()

	if err := watcher.Add(dir); err != nil {
		log.Printf("ui diagnostics: watch %q: %v", dir, err)
		_ = watcher.Close()

		return nil
	}

	diagnostics := &diagnosticsWatcher{
		hub:     hub,
		watcher: watcher,
		pending: make(map[string]*time.Timer),
	}

	go diagnostics.run()

	return diagnostics
}

func (diagnostics *diagnosticsWatcher) Close() error {
	if diagnostics == nil {
		return nil
	}

	diagnostics.pendingMu.Lock()

	for signal, timer := range diagnostics.pending {
		timer.Stop()
		delete(diagnostics.pending, signal)
	}

	diagnostics.pendingMu.Unlock()

	if diagnostics.watcher == nil {
		return nil
	}

	return diagnostics.watcher.Close()
}

func (diagnostics *diagnosticsWatcher) run() {
	for {
		select {
		case <-diagnostics.hub.ctx.Done():
			return
		case event, ok := <-diagnostics.watcher.Events:
			if !ok {
				return
			}

			if !diagnostics.handlesEvent(event.Name) {
				continue
			}

			signal := signalFromDumpPath(event.Name)

			if signal == "" {
				diagnostics.publishDumpList()

				continue
			}

			diagnostics.schedule(signal)
		case err, ok := <-diagnostics.watcher.Errors:
			if !ok {
				return
			}

			log.Printf("ui diagnostics: watcher: %v", err)
		}
	}
}

func (diagnostics *diagnosticsWatcher) handlesEvent(name string) bool {
	base := filepath.Base(name)

	if strings.HasSuffix(base, rawDumpSuffix) {
		return true
	}

	if strings.Contains(base, "_raw.jsonl") {
		return true
	}

	return filepath.Dir(name) == rawdump.Dir()
}

func signalFromDumpPath(name string) string {
	base := filepath.Base(name)

	if !strings.HasSuffix(base, rawDumpSuffix) {
		return ""
	}

	signal := strings.TrimSuffix(base, rawDumpSuffix)

	if !validSignalName(signal) {
		return ""
	}

	return signal
}

func (diagnostics *diagnosticsWatcher) schedule(signal string) {
	diagnostics.pendingMu.Lock()
	defer diagnostics.pendingMu.Unlock()

	if timer := diagnostics.pending[signal]; timer != nil {
		timer.Stop()
	}

	diagnostics.pending[signal] = time.AfterFunc(diagnosticsDebounce, func() {
		diagnostics.pendingMu.Lock()
		delete(diagnostics.pending, signal)
		diagnostics.pendingMu.Unlock()

		if diagnostics.hub.ctx.Err() != nil {
			return
		}

		diagnostics.publishSignal(signal)
		diagnostics.publishDumpList()
	})
}

func (diagnostics *diagnosticsWatcher) publishSignal(signal string) {
	if diagnostics.hub.ctx.Err() != nil {
		return
	}
	path := filepath.Join(rawdump.Dir(), signal+rawDumpSuffix)

	report, err := analyze.AnalyzeFileTail(signal, path, analyze.LiveMaxRows)

	if err != nil {
		log.Printf("ui diagnostics: analyze %s: %v", signal, err)

		return
	}

	payload, err := diagnosticFrame(report)

	if err != nil {
		log.Printf("ui diagnostics: encode %s: %v", signal, err)

		return
	}

	diagnostics.hub.broadcastJSON(payload)
}

func (diagnostics *diagnosticsWatcher) publishDumpList() {
	if diagnostics.hub.ctx.Err() != nil {
		return
	}

	dumps, err := listRawDumps()

	if err != nil {
		log.Printf("ui diagnostics: list dumps: %v", err)

		return
	}

	payload := map[string]any{
		"event": "dumps",
		"dumps": dumps,
	}

	diagnostics.hub.lastDumps.Store(&payload)
	diagnostics.hub.broadcastJSON(payload)
}

func diagnosticFrame(report *analyze.Report) (map[string]any, error) {
	raw, err := sonic.Marshal(report)

	if err != nil {
		return nil, err
	}

	payload := map[string]any{}

	if err := sonic.Unmarshal(raw, &payload); err != nil {
		return nil, err
	}

	payload["chart"] = "diagnostic"

	return payload, nil
}
