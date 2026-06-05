package ui

import (
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/bytedance/sonic"
	"github.com/theapemachine/symm/analyze"
	"github.com/theapemachine/symm/rawdump"
)

const rawDumpSuffix = "_raw.jsonl"

// dumpInfo describes one available raw dump file for the diagnostics frontend.
type dumpInfo struct {
	Signal   string `json:"signal"`
	File     string `json:"file"`
	Bytes    int64  `json:"bytes"`
	Modified string `json:"modified"`
}

/*
handleListDumps reports the raw dump files currently available under runs/, one
per signal, so the diagnostics page can offer them for analysis.
*/
func (hub *Hub) handleListDumps(writer http.ResponseWriter, request *http.Request) {
	applyDiagnosticsCORS(writer, request)

	if request.Method == http.MethodOptions {
		writer.WriteHeader(http.StatusNoContent)

		return
	}

	dir := rawdump.Dir()
	entries, err := os.ReadDir(dir)

	if err != nil {
		writeDiagnosticsJSON(writer, http.StatusOK, map[string]any{"dumps": []dumpInfo{}})

		return
	}

	dumps := make([]dumpInfo, 0, len(entries))

	for _, entry := range entries {
		name := entry.Name()

		if entry.IsDir() || !strings.HasSuffix(name, rawDumpSuffix) {
			continue
		}

		info, err := entry.Info()

		if err != nil {
			continue
		}

		dumps = append(dumps, dumpInfo{
			Signal:   strings.TrimSuffix(name, rawDumpSuffix),
			File:     filepath.Join(dir, name),
			Bytes:    info.Size(),
			Modified: info.ModTime().UTC().Format(time.RFC3339),
		})
	}

	sort.Slice(dumps, func(i, j int) bool {
		return dumps[i].Signal < dumps[j].Signal
	})

	writeDiagnosticsJSON(writer, http.StatusOK, map[string]any{"dumps": dumps})
}

/*
handleAnalyze runs the diagnostic battery over one signal's raw dump and returns
the report. The signal is resolved to runs/<signal>_raw.jsonl after validation, so
the endpoint can never read outside the dumps directory.
*/
func (hub *Hub) handleAnalyze(writer http.ResponseWriter, request *http.Request) {
	applyDiagnosticsCORS(writer, request)

	if request.Method == http.MethodOptions {
		writer.WriteHeader(http.StatusNoContent)

		return
	}

	signal := strings.TrimSpace(request.URL.Query().Get("signal"))

	if !validSignalName(signal) {
		writeDiagnosticsJSON(writer, http.StatusBadRequest, map[string]any{
			"error": "missing or invalid signal parameter",
		})

		return
	}

	path := filepath.Join(rawdump.Dir(), signal+rawDumpSuffix)

	if _, err := os.Stat(path); err != nil {
		writeDiagnosticsJSON(writer, http.StatusNotFound, map[string]any{
			"error": "no dump found for signal " + signal,
		})

		return
	}

	report, err := analyze.AnalyzeFile(signal, path, parseMaxRows(request.URL.Query().Get("max_rows")))

	if err != nil {
		writeDiagnosticsJSON(writer, http.StatusInternalServerError, map[string]any{
			"error": err.Error(),
		})

		return
	}

	writeDiagnosticsJSON(writer, http.StatusOK, report)
}

// validSignalName guards the file lookup: only short lower-case identifiers are
// allowed, so a request can never traverse out of the dumps directory.
func validSignalName(name string) bool {
	if name == "" || len(name) > 64 {
		return false
	}

	for _, r := range name {
		if (r < 'a' || r > 'z') && (r < '0' || r > '9') && r != '_' {
			return false
		}
	}

	return true
}

func parseMaxRows(raw string) int {
	value, err := strconv.Atoi(strings.TrimSpace(raw))

	if err != nil || value < 0 {
		return 0
	}

	return value
}

func writeDiagnosticsJSON(writer http.ResponseWriter, status int, payload any) {
	raw, err := sonic.Marshal(payload)

	if err != nil {
		http.Error(writer, `{"error":"failed to encode response"}`, http.StatusInternalServerError)

		return
	}

	writer.Header().Set("Content-Type", "application/json")
	writer.WriteHeader(status)
	_, _ = writer.Write(raw)
}

// applyDiagnosticsCORS mirrors the websocket origin policy: same-origin and
// localhost callers (the Vite dev server) are allowed, nothing else.
func applyDiagnosticsCORS(writer http.ResponseWriter, request *http.Request) {
	origin := strings.TrimSpace(request.Header.Get("Origin"))

	if origin == "" {
		return
	}

	if !isLocalhostOrigin(origin) {
		return
	}

	writer.Header().Set("Access-Control-Allow-Origin", origin)
	writer.Header().Set("Vary", "Origin")
	writer.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")
	writer.Header().Set("Access-Control-Allow-Headers", "Content-Type")
}

func isLocalhostOrigin(origin string) bool {
	parsed, err := url.Parse(origin)

	if err != nil {
		return false
	}

	host := strings.ToLower(parsed.Hostname())

	return host == "127.0.0.1" || host == "localhost" || host == "::1"
}
