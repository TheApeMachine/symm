package depthflow

import (
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/transport"
	"testing"
)

func TestDepthGraphPrimitiveMessageIsolation(t *testing.T) {
	graph, p := newDepthGraph(), depthProjection()
	for index := 0; index < 4; index++ {
		bid, ask, elapsed := 296.0, 202.0, 0.0
		if index > 0 {
			bid, ask, elapsed = 100, 0, 1
		}
		fields, err := transport.Evaluate[map[string]core.Primitive](graph, core.Record(map[string]any{
			"observed_notional:bid": bid, "observed_notional:ask": ask, "add_notional:bid": bid, "add_notional:ask": ask,
			"modify_remaining_notional:bid": 0.0, "modify_remaining_notional:ask": 0.0, "delete_count:bid": 0.0, "delete_count:ask": 0.0,
			"mutation_count:bid": 2.0, "mutation_count:ask": 1.0, "elapsed": elapsed,
		}))
		if err != nil {
			t.Fatal(err)
		}
		m := p.Project(fields)
		if m.Err != nil {
			t.Fatal(m.Err)
		}
		if m.Metrics["observed_notional"].Raw != bid+ask {
			t.Fatal("inherited prior book", m.Metrics)
		}
		if index == 0 {
			if _, found := m.Metrics["observed_notional_rate"]; found {
				t.Fatal("cold rate")
			}
		} else if m.Metrics["observed_notional_rate"].Raw != 100 {
			t.Fatal("rate lost", m.Metrics)
		}
	}
}
