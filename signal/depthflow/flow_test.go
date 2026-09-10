package depthflow

import (
	"testing"

	"github.com/theapemachine/symm/nomagique/transport"
)

func TestDepthNext(t *testing.T) {
	graph, p := newDepthGraph(), depthProjection()
	for index := 0; index < 4; index++ {
		bid, ask, elapsed := 296.0, 202.0, 0.0
		if index > 0 {
			bid, ask, elapsed = 100, 0, 1
		}
		fields, err := transport.Evaluate(graph, transport.Values(DepthInput{
			ObservedBid: bid, ObservedAsk: ask, AddBid: bid, AddAsk: ask,
			MutationBid: 2, MutationAsk: 1, Elapsed: elapsed,
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
			continue
		}
		if m.Metrics["observed_notional_rate"].Raw != 100 {
			t.Fatal("rate lost", m.Metrics)
		}
	}
}
