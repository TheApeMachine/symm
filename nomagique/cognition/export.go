package cognition

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"iter"
	"math"
	"strings"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
)

/*
Export publishes the dashboard tree for the association trie as JSON text
under "tree". Region frames are the slash-separated context (edges). The
terminal class is the leaf — enter or exit only; wait is never a leaf
(legacy wait basins are skipped). A class whose strength sits above the
graded start, the center of the unit interval, is the policy choice.
Internal region nodes are the precursor stance before an enter/exit leaf.
*/
type Export struct {
	*core.PrimitiveError
	memory *Associate
}

func NewExport(memory *Associate) *Export {
	return &Export{
		PrimitiveError: core.NewPrimitiveError(),
		memory:         memory,
	}
}

func (op *Export) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			if arriving == nil || op.memory == nil {
				op.Error(core.ErrShape)
				return
			}

			adapter := *(**data.Adapter)(arriving)

			if adapter == nil {
				op.Error(core.ErrShape)
				return
			}

			rootTree := op.memory.root.Load()

			if rootTree == nil {
				op.Error(core.ErrShape)
				return
			}

			var classes []string
			var contexts [][]byte
			var probabilities []float64
			var counts []uint64
			var keys [][]byte
			iterator := rootTree.Root().Iterator()

			for key, value, found := iterator.Next(); found; key, value, found = iterator.Next() {
				if len(key) < 4 || key[0] != 'b' || key[1] != '/' || len(value) != 24 {
					continue
				}

				rest := key[2:]
				slash := bytes.LastIndexByte(rest, '/')

				if slash <= 0 || slash == len(rest)-1 {
					continue
				}

				count := binary.LittleEndian.Uint64(value[0:8])

				if count == 0 {
					continue
				}

				classes = append(classes, string(rest[slash+1:]))
				contexts = append(contexts, append([]byte{}, rest[:slash]...))
				probabilities = append(probabilities, math.Float64frombits(binary.LittleEndian.Uint64(value[8:16])))
				counts = append(counts, count)
				keys = append(keys, append([]byte{}, key...))
			}

			root := map[string]any{
				"id":          "root",
				"prefix":      "ROOT",
				"probability": core.Unit,
				"count":       uint64(0),
				"state":       "ESTIMATED",
			}
			branches := make([]any, 0)
			feasible := make([]any, 0)

			if len(classes) > 0 {
				root["state"] = "EVALUATED"
			}

			for index := range classes {
				if classes[index] == "wait" {
					continue
				}

				parts := bytes.Split(contexts[index], []byte{'/'})
				var frames []string
				previous := ""

				for _, part := range parts {
					if len(part) == 0 {
						continue
					}

					frame := string(part)

					if part[0] < 32 {
						var builder strings.Builder

						for itemIndex, item := range part {
							if itemIndex > 0 {
								builder.WriteByte('_')
							}

							builder.WriteString(fmt.Sprintf("R%d", item))
						}

						frame = builder.String()
					}

					if frame == "" || frame == previous {
						continue
					}

					previous = frame
					frames = append(frames, frame)
				}

				parent := root
				var path strings.Builder
				path.WriteString("root")
				probability := probabilities[index]
				count := counts[index]

				for _, frame := range frames {
					path.WriteByte('/')
					path.WriteString(frame)
					children, _ := parent["children"].([]any)
					var found map[string]any

					for _, child := range children {
						node, _ := child.(map[string]any)

						if node == nil {
							continue
						}

						if leaf, _ := node["leaf"].(bool); leaf {
							continue
						}

						if node["prefix"] == frame {
							found = node
							break
						}
					}

					if found == nil {
						found = map[string]any{
							"id":              path.String(),
							"prefix":          frame,
							"probability":     probability,
							"stepProbability": probability,
							"count":           uint64(0),
							"tokens":          []string{frame},
							"state":           "EVALUATED",
						}
						children = append(children, found)
						parent["children"] = children
					}

					found["count"] = found["count"].(uint64) + count
					current, _ := found["probability"].(float64)

					if probability > current {
						found["probability"] = probability
						found["stepProbability"] = probability
					}

					parent = found
				}

				actionName := strings.ToUpper(classes[index])
				path.WriteByte('/')
				path.WriteString(actionName)
				policy := "EVALUATED"

				if probability > core.Unit/2 {
					policy = "POLICY CHOICE"
				}

				children, _ := parent["children"].([]any)
				var action map[string]any

				for _, child := range children {
					node, _ := child.(map[string]any)

					if node == nil {
						continue
					}

					if leaf, _ := node["leaf"].(bool); leaf && node["prefix"] == actionName {
						action = node
						break
					}
				}

				if action == nil {
					action = map[string]any{
						"id":              path.String(),
						"prefix":          actionName,
						"probability":     probability,
						"stepProbability": probability,
						"count":           uint64(0),
						"tokens":          []string{classes[index]},
						"state":           policy,
						"leaf":            true,
					}
					children = append(children, action)
					parent["children"] = children
				}

				action["count"] = action["count"].(uint64) + count
				current, _ := action["probability"].(float64)

				if probability >= current {
					action["probability"] = probability
					action["stepProbability"] = probability
					action["state"] = policy
				}

				branches = append(branches, map[string]any{
					"hash":       fmt.Sprintf("0x%x:%s", keys[index], classes[index]),
					"depth":      len(frames) + 1,
					"visits":     count,
					"meanEdge":   probability - core.Unit/2,
					"confidence": probability * 100,
					"policy":     actionName,
				})
				feasible = append(feasible, map[string]any{
					"action":      classes[index],
					"prefix":      fmt.Sprintf("ROOT / [%s] -> %s", strings.Join(frames, ", "), classes[index]),
					"probability": probability,
					"state":       policy,
				})
			}

			for index := range feasible {
				node, _ := feasible[index].(map[string]any)

				if node == nil {
					continue
				}

				node["rank"] = index + 1
			}

			stack := []map[string]any{root}

			for len(stack) > 0 {
				node := stack[len(stack)-1]
				stack = stack[:len(stack)-1]
				delete(node, "leaf")
				children, _ := node["children"].([]any)

				for _, child := range children {
					next, _ := child.(map[string]any)

					if next == nil {
						continue
					}

					stack = append(stack, next)
				}
			}

			encoded, err := json.Marshal(map[string]any{
				"root":     root,
				"branches": branches,
				"feasible": feasible,
			})

			if err != nil {
				op.Error(err)
				return
			}

			published := data.NewTextMap()
			published.Values["tree"] = string(encoded)

			for range adapter.Next(data.NewValue(published)) {
			}

			if err := adapter.Error(); err != nil {
				op.Error(err)
				return
			}

			if !yield(arriving) {
				return
			}
		}
	}
}
