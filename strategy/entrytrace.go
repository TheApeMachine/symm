package strategy

import (
	"encoding/json"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"github.com/theapemachine/nomagique/runtime"
	"github.com/theapemachine/nomagique/store"
)

/*
EntryTrace appends one NDJSON line per decision so a stuck desk can be
diagnosed from disk without staring at the UI. Enable with SYMM_ENTRY_LOG
(path); default is entry-trace.jsonl in the process working directory.
Set SYMM_ENTRY_LOG=off to disable.
*/
type EntryTrace struct {
	mu      sync.Mutex
	file    *os.File
	path    string
	disabled bool
	total   atomic.Uint64
	gates   sync.Map // gate|action → *atomic.Uint64
}

var entryTrace = newEntryTrace()

func newEntryTrace() *EntryTrace {
	path := os.Getenv("SYMM_ENTRY_LOG")

	switch path {
	case "off", "0", "false":
		return &EntryTrace{disabled: true}
	case "":
		path = "entry-trace.jsonl"
	}

	file, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return &EntryTrace{disabled: true, path: path}
	}

	trace := &EntryTrace{file: file, path: path}
	trace.write(map[string]any{
		"ts":   time.Now().UTC().Format(time.RFC3339Nano),
		"type": "open",
		"path": path,
	})

	return trace
}

/*
NoteDecision records one model/board/tier outcome. Oracle skips include the
per-gate side map so a red lamp can be named without replaying the tick.
*/
func NoteDecision(
	decision *runtime.Decision,
	regions []*runtime.Region,
	extra map[string]any,
) {
	entryTrace.note(decision, regions, extra)
}

func (trace *EntryTrace) note(
	decision *runtime.Decision,
	regions []*runtime.Region,
	extra map[string]any,
) {
	if trace == nil || trace.disabled || decision == nil {
		return
	}

	gate := decision.Gate
	if gate == "" {
		gate = decision.Action
	}

	key := gate + "|" + decision.Action
	counter, _ := trace.gates.LoadOrStore(key, &atomic.Uint64{})
	counter.(*atomic.Uint64).Add(1)
	n := trace.total.Add(1)

	row := map[string]any{
		"ts":     time.Now().UTC().Format(time.RFC3339Nano),
		"type":   "decision",
		"symbol": decision.Symbol,
		"action": decision.Action,
		"gate":   decision.Gate,
		"reason": decision.Reason,
		"path":   decision.Path,
		"tier":   decision.Tier,
		"n":      n,
	}

	if decision.Gate == "oracle" || decision.Action == "enter" {
		if sides := oracleSides(regions); len(sides) > 0 {
			row["oracles"] = sides
		}
	}

	for k, v := range extra {
		if k == "" || v == nil {
			continue
		}

		row[k] = v
	}

	trace.write(row)

	if n%500 == 0 {
		trace.summary()
	}
}

func (trace *EntryTrace) summary() {
	gates := map[string]uint64{}

	trace.gates.Range(func(key, value any) bool {
		gates[key.(string)] = value.(*atomic.Uint64).Load()
		return true
	})

	trace.write(map[string]any{
		"ts":    time.Now().UTC().Format(time.RFC3339Nano),
		"type":  "summary",
		"n":     trace.total.Load(),
		"gates": gates,
		"path":  trace.path,
	})
}

func (trace *EntryTrace) write(row map[string]any) {
	payload, err := json.Marshal(row)
	if err != nil {
		return
	}

	trace.mu.Lock()
	defer trace.mu.Unlock()

	if trace.file == nil {
		return
	}

	_, _ = trace.file.Write(append(payload, '\n'))
}

/*
oracleSides names each oracle pair's emitted side and brightness for the log.
*/
func oracleSides(regions []*runtime.Region) map[string]any {
	if len(regions) == 0 {
		return nil
	}

	out := make(map[string]any, 8)

	for _, region := range regions {
		if region == nil || region.Key < 9 {
			continue
		}

		name := oracleGateID(region.Key)
		if name == "" {
			continue
		}

		side := store.Catalog.ByKey(region.Key).Side
		out[name] = map[string]any{
			"side":       side,
			"key":        region.Key,
			"brightness": region.Brightness,
		}
	}

	return out
}

func oracleGateID(key uint8) string {
	switch key {
	case 9, 10:
		return "vol"
	case 11, 12:
		return "day"
	case 13, 14:
		return "spr"
	case 15, 16:
		return "bok"
	case 17, 18:
		return "brd"
	case 19, 20:
		return "ctg"
	case 21, 22:
		return "cap"
	case 23, 24:
		return "pk"
	default:
		return ""
	}
}
