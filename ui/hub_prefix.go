package ui

import (
	"fmt"
	"strings"
	"sync"

	"github.com/bytedance/sonic"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/nomagique/data"
	"github.com/theapemachine/symm/system"
)

/*
pathsListing is the JSON body under envelope key "paths" for model/paths.
Thresholds are absolute desk books from PathBook JSON; keys end in
enter|hold|exit.bin so the trie needs no blob decode on the wire.
*/
type pathsListing struct {
	Scope      string             `json:"scope"`
	Epoch      string             `json:"epoch,omitempty"`
	Thresholds map[string]float64 `json:"thresholds"`
}

/*
pathBook mirrors strategy.PathBook JSON without importing strategy
(strategy → broker → ui would otherwise cycle).
*/
type pathBook struct {
	Action     string  `json:"action"`
	Resistance float64 `json:"resistance"`
}

/*
closedListing is the JSON body under envelope key "closed" for
paper|real|virtual/closed history.
*/
type closedListing struct {
	Scope   string        `json:"scope"`
	Entries []closedEntry `json:"entries"`
}

type closedEntry struct {
	Epoch     uint64         `json:"epoch"`
	Tick      uint64         `json:"tick"`
	Timestamp uint64         `json:"timestamp"`
	Row       map[string]any `json:"row"`
}

/*
read fills one closed position from a stored JSON object.
Canonical objects are {epoch,tick,timestamp,position:{…}}. Legacy bare
*Position JSON (id/symbol/tier_key at the top level) is still accepted.
Missing epoch, tick, and timestamp stay zero. book is filled from tier_key
when the store row only carried the broker field.
*/
func (entry *closedEntry) read(raw []byte) bool {
	if len(raw) == 0 {
		return false
	}

	row, err := data.AttributeOk[map[string]any](raw, "position")

	if err != nil || len(row) == 0 {
		var top map[string]any
		if uerr := sonic.Unmarshal(raw, &top); uerr != nil || len(top) == 0 {
			return false
		}
		if _, wrapped := top["position"]; wrapped {
			return false
		}
		id, _ := top["id"].(string)
		if id == "" {
			return false
		}
		row = top
		entry.Epoch = 0
		entry.Tick = 0
		entry.Timestamp = 0
	} else {
		id, _ := row["id"].(string)
		if id == "" {
			return false
		}
		entry.Epoch = data.Attribute[uint64](raw, "epoch")
		entry.Tick = data.Attribute[uint64](raw, "tick")
		entry.Timestamp = data.Attribute[uint64](raw, "timestamp")
	}

	if book, _ := row["book"].(string); book == "" {
		if tk, ok := row["tier_key"].(string); ok && tk != "" {
			row["book"] = tk
		}
	}

	entry.Row = row
	return true
}

/*
ListPrefix lists object keys under prefix and writes one or more JSON
envelopes that stay under the websocket max message size.

model/paths → {"pathbook":{scope,epoch,thresholds}}
paper|real|virtual/closed → {"closed":{scope,entries}}

Path leaves are PathBook JSON. Closed positions are JSON with a position object.
Live sequencer Tokens still publish under "paths" — a different key.
*/
func (hub *Hub) ListPrefix(prefix string) {
	if hub.store == nil || prefix == "" {
		return
	}

	budget := listFrameBudget()

	switch {
	case strings.HasPrefix(prefix, "model/paths"):
		hub.writePathsListing(prefix, budget)
	case closedHistoryPrefix(prefix):
		hub.writeClosedListing(prefix, budget)
	default:
		errnie.Warn(fmt.Sprintf("[hub] ListPrefix unsupported prefix %q", prefix))
	}
}

func closedHistoryPrefix(prefix string) bool {
	for _, stem := range []string{
		"paper/closed",
		"real/closed",
		"virtual/closed",
	} {
		if strings.HasPrefix(prefix, stem) {
			return true
		}
	}

	return false
}

func (hub *Hub) writePathsListing(prefix string, budget int) {
	thresholds := make(map[string]float64, 256)

	for _, item := range hub.downloadPrefix(prefix) {
		book := decodePathBook(item.raw)

		if book == nil {
			continue
		}

		if book.Resistance > 0.8 {
			continue
		}

		thresholds[policyPathKey(item.key, book.Action)] = book.Resistance
	}

	if len(thresholds) == 0 {
		return
	}

	for _, chunk := range chunkThresholds(thresholds, budget) {
		hub.WriteJSON(map[string]any{
			"pathbook": pathsListing{
				Scope:      "model/paths",
				Thresholds: chunk,
			},
		})
	}
}

func decodePathBook(raw []byte) *pathBook {
	if len(raw) == 0 {
		return nil
	}

	var book pathBook

	if err := sonic.Unmarshal(raw, &book); err != nil {
		return nil
	}

	if book.Action == "" {
		return nil
	}

	return &book
}

type prefixObject struct {
	key string
	raw []byte
}

/*
downloadPrefix lists keys then fetches bodies through a bounded worker pool so
inspecting model/paths/ or closed/ cannot open thousands of S3 GETs at once.
*/
func (hub *Hub) downloadPrefix(prefix string) []prefixObject {
	keys := make([]string, 0, 256)

	for key := range hub.store.ListKeys(prefix) {
		keys = append(keys, key)
	}

	out := make([]prefixObject, len(keys))
	workers := 32
	if workers > len(keys) {
		workers = len(keys)
	}
	if workers <= 0 {
		return nil
	}

	jobs := make(chan int, workers)
	var wg sync.WaitGroup

	for range workers {
		wg.Go(func() {
			for i := range jobs {
				key := keys[i]
				raw, err := hub.store.Download(hub.ctx, key)
				if err != nil {
					errnie.Error(errnie.Err(
						errnie.IO, "[hub] error downloading prefix object", err,
					))
					continue
				}
				out[i] = prefixObject{key: key, raw: raw}
			}
		})
	}

	for i := range keys {
		jobs <- i
	}
	close(jobs)
	wg.Wait()

	kept := make([]prefixObject, 0, len(out))

	for _, item := range out {
		if item.key == "" || len(item.raw) == 0 {
			continue
		}

		kept = append(kept, item)
	}

	return kept
}

func (hub *Hub) writeClosedListing(prefix string, budget int) {
	entries := make([]closedEntry, 0, 64)

	for _, item := range hub.downloadPrefix(prefix) {
		var entry closedEntry

		if !entry.read(item.raw) {
			continue
		}

		entries = append(entries, entry)
	}

	if len(entries) == 0 {
		return
	}

	for _, chunk := range chunkClosedEntries(entries, budget) {
		hub.WriteJSON(map[string]any{
			"closed": closedListing{
				Scope:   closedListingScope(prefix),
				Entries: chunk,
			},
		})
	}
}

func closedListingScope(prefix string) string {
	switch {
	case strings.HasPrefix(prefix, "virtual/closed"):
		return "virtual/closed"
	case strings.HasPrefix(prefix, "real/closed"):
		return "real/closed"
	case strings.HasPrefix(prefix, "paper/closed"):
		return "paper/closed"
	default:
		return "paper/closed"
	}
}

func listFrameBudget() int {
	return system.Cfg.UI.WebSocket.MaxMessageBytes * 9 / 10
}

/*
policyPathKey is the pathbook trie id the Cognitive viz expects:
model/paths/<tokens>/<enter|hold|exit>.bin. Storage leaves are flat
model/paths/<tokens>.bin with Action inside the PathBook JSON — rewrite so
terminal nodes paint ENTER/HOLD/EXIT instead of the last region token.
*/
func policyPathKey(storageKey, action string) string {
	switch {
	case strings.HasSuffix(storageKey, "/enter.bin"),
		strings.HasSuffix(storageKey, "/hold.bin"),
		strings.HasSuffix(storageKey, "/exit.bin"):
		return storageKey
	}

	switch action {
	case "enter", "hold", "exit":
		base := strings.TrimSuffix(storageKey, ".bin")
		return base + "/" + action + ".bin"
	default:
		return storageKey
	}
}

func chunkThresholds(
	all map[string]float64,
	budget int,
) []map[string]float64 {
	if len(all) == 0 || budget <= 0 {
		return nil
	}

	chunks := make([]map[string]float64, 0, 1)
	current := make(map[string]float64)
	used := 64

	flush := func() {
		if len(current) == 0 {
			return
		}

		chunks = append(chunks, current)
		current = make(map[string]float64)
		used = 64
	}

	for key, value := range all {
		cost := len(key) + 24

		if len(current) > 0 && used+cost > budget {
			flush()
		}

		current[key] = value
		used += cost
	}

	flush()

	return chunks
}

func chunkClosedEntries(entries []closedEntry, budget int) [][]closedEntry {
	if len(entries) == 0 || budget <= 0 {
		return nil
	}

	chunks := make([][]closedEntry, 0, 1)
	start := 0
	used := 64

	flush := func(end int) {
		if end <= start {
			return
		}

		chunks = append(chunks, entries[start:end])
		start = end
		used = 64
	}

	for i, entry := range entries {
		raw, err := Sonic.Marshal(entry)
		cost := 64

		if err == nil {
			cost = len(raw) + 8
		}

		if i > start && used+cost > budget {
			flush(i)
		}

		if cost > budget {
			errnie.Warn(fmt.Sprintf(
				"[hub] skipping oversized closed position (%d bytes > budget %d)",
				cost, budget,
			))
			start = i + 1
			used = 64
			continue
		}

		used += cost
	}

	flush(len(entries))

	return chunks
}
