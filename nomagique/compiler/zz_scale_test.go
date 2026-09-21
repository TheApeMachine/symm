package compiler

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"
)

// Build a graph with N independent extract->insert chains to measure how
// per-evaluation cost scales with node count.
func buildGraph(chains int) Graph {
	nodes := map[string]Node{}
	nodes["sink"] = Node{ID: "sink", Type: "sink",
		Connections: Connections{Inputs: map[string][]ConnectionTarget{}, Outputs: map[string][]ConnectionTarget{}}}
	for i := 0; i < chains; i++ {
		ex := fmt.Sprintf("ex%d", i)
		ins := fmt.Sprintf("ins%d", i)
		nodes[ex] = Node{ID: ex, Type: "data.Extract",
			InputData: map[string]json.RawMessage{"path": json.RawMessage(`"a.b"`)},
			Connections: Connections{Inputs: map[string][]ConnectionTarget{},
				Outputs: map[string][]ConnectionTarget{"out": {{NodeID: ins, PortName: "value"}}}}}
		nodes[ins] = Node{ID: ins, Type: "data.Insert",
			InputData: map[string]json.RawMessage{"path": json.RawMessage(`"m"`)},
			Connections: Connections{Inputs: map[string][]ConnectionTarget{},
				Outputs: map[string][]ConnectionTarget{"out": {{NodeID: "sink", PortName: "value"}}}}}
	}
	return Graph{ID: "scale", Name: "scale", Nodes: nodes}
}

func TestScaling(t *testing.T) {
	ctx := context.Background()
	for _, chains := range []int{10, 100, 500, 1000} {
		p, err := Compile(buildGraph(chains), nil, DefaultRepository())
		if err != nil {
			t.Fatal(err)
		}
		// warm
		p.Execute(ctx, nil)
		start := time.Now()
		const iters = 20
		for i := 0; i < iters; i++ {
			if err := p.Execute(ctx, nil); err != nil {
				t.Fatal(err)
			}
		}
		per := time.Since(start) / iters
		t.Logf("nodes=%5d  per-evaluation=%v  => max evals/sec=%.0f", len(p.Nodes), per, float64(time.Second)/float64(per))
		p.Release()
	}
}
