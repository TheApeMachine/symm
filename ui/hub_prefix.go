package ui

import (
	"fmt"
	"strings"

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
read fills one closed lot from a stored JSON object.
Missing epoch, tick, and timestamp stay zero.
*/
func (entry *closedEntry) read(raw []byte) bool {
	if entry == nil || len(raw) == 0 {
		return false
	}

	row, err := data.AttributeOk[map[string]any](raw, "position")

	if err != nil || len(row) == 0 {
		return false
	}

	id, _ := row["id"].(string)

	if id == "" {
		return false
	}

	entry.Epoch = data.Attribute[uint64](raw, "epoch")
	entry.Tick = data.Attribute[uint64](raw, "tick")
	entry.Timestamp = data.Attribute[uint64](raw, "timestamp")
	entry.Row = row

	return true
}

/*
ListPrefix lists object keys under prefix and writes one or more JSON
envelopes that stay under the websocket max message size.

model/paths → {"paths":{scope,epoch,thresholds}}
paper|real|virtual/closed → {"closed":{scope,entries}}

Path leaves are PathBook JSON. Closed lots are JSON with a position object.
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

	for key := range hub.store.ListKeys(prefix) {
		raw, err := hub.store.Download(hub.ctx, key)

		if err != nil {
			errnie.Error(errnie.Err(
				errnie.IO, "[hub] error downloading path book", err,
			))

			continue
		}

		book := decodePathBook(raw)

		if book == nil {
			continue
		}

		if book.Resistance > 0.8 {
			continue
		}

		thresholds[policyPathKey(key, book.Action)] = book.Resistance
	}

	if len(thresholds) == 0 {
		return
	}

	for _, chunk := range chunkThresholds(thresholds, budget) {
		hub.WriteJSON(map[string]any{
			"paths": pathsListing{
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

func (hub *Hub) writeClosedListing(prefix string, budget int) {
	entries := make([]closedEntry, 0, 64)

	for key := range hub.store.ListKeys(prefix) {
		var entry closedEntry

		raw, err := hub.store.Download(hub.ctx, key)

		if err != nil {
			errnie.Error(errnie.Err(
				errnie.IO, "[hub] error downloading closed entry", err,
			))

			continue
		}

		if !entry.read(raw) {
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
	maxBytes := 4 << 20

	if system.Cfg != nil && system.Cfg.UI != nil && system.Cfg.UI.WebSocket != nil {
		if configured := system.Cfg.UI.WebSocket.MaxMessageBytes; configured > 0 {
			maxBytes = configured
		}
	}

	return maxBytes * 9 / 10
}

/*
policyPathKey normalizes an S3 object key to the trie key the dashboard
expects (…/enter.bin|hold.bin|exit.bin). Tree.persist writes
model/paths/<tokens>.bin with action only inside JSON — rewrite that into
the action-suffixed form the cognitive trie filters on.
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
				"[hub] skipping oversized closed lot (%d bytes > budget %d)",
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
