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
Thresholds are already desk books (0.5 + Σ resistance); keys end in
enter|hold|exit.bin so the trie needs no Value decode on the wire.
*/
type pathsListing struct {
	Scope      string             `json:"scope"`
	Epoch      string             `json:"epoch,omitempty"`
	Thresholds map[string]float64 `json:"thresholds"`
}

/*
closedListing is the JSON body under envelope key "closed" for
position/closed and virtual/closed history.
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
ListPrefix lists object keys under prefix and writes one or more JSON
envelopes that stay under the websocket max message size.

model/paths → {"paths":{scope,epoch,thresholds}}
position|virtual/closed → {"closed":{scope,entries}}

Stored S3 objects may still be data.Value blobs; the dashboard never sees
them — only the compact JSON listing.
*/
func (hub *Hub) ListPrefix(prefix string) {
	if hub.store == nil || prefix == "" {
		return
	}

	budget := listFrameBudget()

	switch {
	case strings.HasPrefix(prefix, "model/paths"):
		hub.writePathsListing(prefix, budget)
	case strings.HasPrefix(prefix, "position/closed"),
		strings.HasPrefix(prefix, "virtual/closed"):
		hub.writeClosedListing(prefix, budget)
	default:
		errnie.Warn(fmt.Sprintf("[hub] ListPrefix unsupported prefix %q", prefix))
	}
}

func (hub *Hub) writePathsListing(prefix string, budget int) {
	thresholds := make(map[string]float64, 256)
	epoch := ""

	for key := range hub.store.ListKeys(prefix) {
		out := hub.store.Download(hub.ctx, key)
		if out == nil {
			continue
		}

		policyKey := policyPathKey(key, out.Scope)
		book := exploreBookThreshold(out)

		if book > 0.8 {
			continue
		}

		thresholds[policyKey] = book

		if epoch == "" && out.Epoch != 0 {
			epoch = fmt.Sprintf("%d", out.Epoch)
		}
	}

	if len(thresholds) == 0 {
		return
	}

	for _, chunk := range chunkThresholds(thresholds, budget) {
		hub.WriteJSON(map[string]any{
			"paths": pathsListing{
				Scope:      "model/paths",
				Epoch:      epoch,
				Thresholds: chunk,
			},
		})
	}
}

func (hub *Hub) writeClosedListing(prefix string, budget int) {
	entries := make([]closedEntry, 0, 64)

	for key := range hub.store.ListKeys(prefix) {
		out := hub.store.Download(hub.ctx, key)
		if out == nil {
			continue
		}

		row := data.Get[map[string]any](out, "position")
		if len(row) == 0 {
			continue
		}

		if id, _ := row["id"].(string); id == "" {
			continue
		}

		entries = append(entries, closedEntry{
			Epoch:     out.Epoch,
			Tick:      out.Tick,
			Timestamp: out.Timestamp,
			Row:       row,
		})
	}

	if len(entries) == 0 {
		return
	}

	scope := "position/closed"
	if strings.HasPrefix(prefix, "virtual/closed") {
		scope = "virtual/closed"
	}

	for _, chunk := range chunkClosedEntries(entries, budget) {
		hub.WriteJSON(map[string]any{
			"closed": closedListing{
				Scope:   scope,
				Entries: chunk,
			},
		})
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
exploreBookThreshold mirrors the frontend desk book: seed 0.5 plus summed
resistance deltas written by strategy/tree.
*/
func exploreBookThreshold(value *data.Value) float64 {
	if value == nil {
		return 0.5
	}

	return 0.5 + value.GetTotal("resistance")
}

/*
policyPathKey normalizes an S3 object key to the trie key the dashboard
expects (…/enter.bin|hold.bin|exit.bin).
*/
func policyPathKey(storageKey, scope string) string {
	if strings.HasSuffix(storageKey, ".bin") {
		return storageKey
	}

	switch scope {
	case "enter", "hold", "exit":
		return storageKey + "/" + scope + ".bin"
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
		raw, err := sonic.Marshal(entry)
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
