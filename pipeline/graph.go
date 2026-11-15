package pipeline

import (
	"sync"
	"sync/atomic"
	"time"

	nmruntime "github.com/theapemachine/nomagique/runtime"
	"github.com/theapemachine/symm/ui"
)

/*
Graph holds probed pipeline nodes plus a synthetic ingress timer.
Measurements publish live via diag_pulse; PublishTopology only ships the
static node/edge skeleton when a dashboard attaches.
*/
type Graph struct {
	mu      sync.Mutex
	order   []string
	byGroup map[string][]*Probe
	ingress stats
}

type stats struct {
	n      atomic.Uint64
	lastNs atomic.Int64
	emaNs  atomic.Int64
	seenAt atomic.Int64
}

/*
NewGraph builds an empty pipeline graph and registers structural pulses.
*/
func NewGraph() *Graph {
	Define("trade", "kraken")
	Define("level3", "kraken")
	Define("rest", "kraken")
	Define("grid", "training")
	Define("sequencer", "training")
	Define("tree", "model")
	Define("board", "model")
	Define("s3", "store")
	Define("queue", "store")

	return &Graph{
		byGroup: make(map[string][]*Probe),
	}
}

/*
Wrap instruments every node in a ring and returns probes as Node values for
Workspace construction.
*/
func (graph *Graph) Wrap(group string, nodes []nmruntime.Node) []nmruntime.Node {
	graph.mu.Lock()
	defer graph.mu.Unlock()

	if _, ok := graph.byGroup[group]; !ok {
		graph.order = append(graph.order, group)
	}

	out := make([]nmruntime.Node, 0, len(nodes))

	for _, node := range nodes {
		probe := NewProbe(group, node)
		graph.byGroup[group] = append(graph.byGroup[group], probe)
		out = append(out, probe)
	}

	return out
}

/*
Count returns completed samples for a probed node or the synthetic ingress
meter. Missing ids return 0 so stall watches can compare without nil checks.
*/
func (graph *Graph) Count(id string) uint64 {
	if id == "" {
		return 0
	}

	if id == "ingress" {
		return graph.ingress.n.Load()
	}

	graph.mu.Lock()
	defer graph.mu.Unlock()

	for _, probes := range graph.byGroup {
		for _, probe := range probes {
			if probe.Name() == id {
				return probe.Samples()
			}
		}
	}

	return 0
}

/*
Ingress records time from trade callback entry through Workspace.Step return
and immediately pulses the dashboard.
*/
func (graph *Graph) Ingress(d time.Duration) {
	ns := d.Nanoseconds()
	if ns < 0 {
		ns = 0
	}

	n := graph.ingress.n.Add(1)
	graph.ingress.lastNs.Store(ns)
	graph.ingress.seenAt.Store(time.Now().UnixNano())

	prev := graph.ingress.emaNs.Load()
	if prev == 0 {
		graph.ingress.emaNs.Store(ns)
	} else {
		graph.ingress.emaNs.Store(prev - prev/8 + ns/8)
	}

	Fire("ingress", map[string]any{
		"n":       n,
		"last_ns": ns,
		"ema_ns":  graph.ingress.emaNs.Load(),
	})
}

/*
PublishTopology pushes the static graph skeleton (no measurement samples).
*/
func (graph *Graph) PublishTopology() {
	if ui.UI == nil {
		return
	}

	// Attach callbacks run after connected=true; still publish during the
	// post-attach grace so a refresh never misses the skeleton.
	ui.UI.WriteJSON(map[string]any{
		"pipeline": graph.Topology(),
	})
}

/*
Topology is id/parent/edge structure only — live values arrive as diag_pulse.
*/
func (graph *Graph) Topology() map[string]any {
	graph.mu.Lock()
	defer graph.mu.Unlock()

	nodes := make([]map[string]any, 0, 48)
	nodes = append(nodes,
		skeleton("kraken", "", "group"),
		skeleton("store", "", "group"),
		skeleton("ingress", "", ""),
	)

	signalIDs := make([]string, 0, 16)
	var resonanceID, trainingID, modelID, persistID string

	for _, group := range graph.order {
		compound := ""
		if group == "signal" || group == "resonance" || group == "training" {
			compound = "ring-" + group
			nodes = append(nodes, skeleton(compound, "", "group"))
		}

		for _, probe := range graph.byGroup[group] {
			id := probe.Name()
			nodes = append(nodes, skeleton(id, compound, ""))

			switch group {
			case "signal":
				signalIDs = append(signalIDs, id)
			case "resonance":
				resonanceID = id
			case "training":
				trainingID = id
			case "model":
				modelID = id
			case "persist":
				persistID = id
			}
		}
	}

	seen := make(map[string]struct{}, len(nodes))
	for _, node := range nodes {
		seen[node["id"].(string)] = struct{}{}
	}

	for _, pulse := range pulseSkeletons() {
		id := pulse["id"].(string)
		if _, ok := seen[id]; ok {
			continue
		}

		nodes = append(nodes, pulse)
	}

	hub := "idle"
	if ui.UI != nil && ui.UI.Active() {
		hub = "ok"
	}

	nodes = append(nodes, map[string]any{
		"id":     "hub",
		"kind":   "sink",
		"status": hub,
	})

	edges := []map[string]string{
		{"from": "trade", "to": "ingress"},
		{"from": "level3", "to": "ingress"},
	}

	for _, id := range signalIDs {
		edges = append(edges, map[string]string{"from": "ingress", "to": id})
		if resonanceID != "" {
			edges = append(edges, map[string]string{"from": id, "to": resonanceID})
		}
	}

	if resonanceID != "" && trainingID != "" {
		edges = append(edges, map[string]string{"from": resonanceID, "to": trainingID})
	}

	if trainingID != "" && modelID != "" {
		edges = append(edges, map[string]string{"from": trainingID, "to": modelID})
	} else if resonanceID != "" && modelID != "" {
		edges = append(edges, map[string]string{"from": resonanceID, "to": modelID})
	}

	if modelID != "" && persistID != "" {
		edges = append(edges, map[string]string{"from": modelID, "to": persistID})
	}

	if persistID != "" {
		edges = append(edges, map[string]string{"from": persistID, "to": "store"})
	}

	if modelID != "" {
		edges = append(edges, map[string]string{"from": modelID, "to": "hub"})
	}

	return map[string]any{
		"nodes": nodes,
		"edges": edges,
		"hub":   hub,
	}
}

func skeleton(id, parent, kind string) map[string]any {
	row := map[string]any{
		"id":     id,
		"status": "ok",
	}

	if parent != "" {
		row["parent"] = parent
	}

	if kind != "" {
		row["kind"] = kind
	}

	return row
}
