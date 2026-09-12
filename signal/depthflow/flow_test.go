package depthflow

import (
	"testing"

	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/transport"
)

func TestDepthNext(t *testing.T) {
	graph, p := newDepthGraph(), depthProjection()
	for index := 0; index < 4; index++ {
		bid, ask, elapsed := 296.0, 202.0, 0.0
		if index > 0 {
			bid, ask, elapsed = 100, 0, 1
		}
		fieldsEval := transport.NewEvaluate(graph)
		var fields data.ProjectionInput

		for out := range fieldsEval.Next(transport.NewValues(DepthInput{
			ObservedBid: bid, ObservedAsk: ask, AddBid: bid, AddAsk: ask,
			MutationBid: 2, MutationAsk: 1, Elapsed: elapsed,
		}).Next(nil)) {
			fields = *(*data.ProjectionInput)(out)
		}

		err := fieldsEval.Error()
		if err != nil {
			t.Fatal(err)
		}
		projectEval := transport.NewEvaluate(p)
		var m *data.Measurement[float64]

		for out := range projectEval.Next(transport.NewValues(fields).Next(nil)) {
			m = *(**data.Measurement[float64])(out)
		}

		if err := projectEval.Error(); err != nil {
			t.Fatal(err)
		}
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
