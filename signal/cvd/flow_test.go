package cvd

import (
	"math"
	"testing"

	"github.com/theapemachine/symm/nomagique/transport"
)

func TestFlowNext(t *testing.T) {
	graph, p := newFlowGraph(), flowProjection()
	for index, event := range []struct {
		buy                              bool
		quantity, seconds, net, baseline float64
	}{
		{true, 2, 0, 200, 0}, {false, 1, 1, 100, 1}, {true, 1, 3, 200, 2.0 / 3},
	} {
		fields, err := transport.Evaluate(graph, transport.Values(FlowInput{
			Price: 100, Quantity: event.quantity, Buy: event.buy,
			At: int64(event.seconds * 1e9),
		}))
		if err != nil {
			t.Fatal(err)
		}
		m := p.Project(fields)
		if m.Err != nil {
			t.Fatal(m.Err)
		}
		if m.Metrics["net_notional"].Raw != event.net || m.Metrics["trade_count"].Raw != float64(index+1) {
			t.Fatalf("accounting: %+v", m.Metrics)
		}
		if index == 0 {
			if _, found := m.Metrics["trade_rate"]; found {
				t.Fatal("cold rate")
			}
			if _, found := m.Metrics["signed_net_fraction_baseline"]; found {
				t.Fatal("cold baseline")
			}
			if m.SNRDefined || m.Maturity != 0 {
				t.Fatalf("cold evidence: %+v", m)
			}
			continue
		}
		if math.Abs(m.Metrics["signed_net_fraction_baseline"].Raw-event.baseline) > 1e-12 {
			t.Fatalf("baseline: %+v", m)
		}
	}
}

func TestFlowQuotedResponse(t *testing.T) {
	graph, p := newFlowGraph(), flowProjection()
	for index := 0; index < 3; index++ {
		fields, err := transport.Evaluate(graph, transport.Values(FlowInput{
			Price: 100, Quantity: 1, Buy: index != 1, At: int64(index) * 1e9,
			Quoted: index > 0, Midpoint: 102, PriorMid: 101,
		}))
		if err != nil {
			t.Fatal(err)
		}
		m := p.Project(fields)
		if m.Err != nil {
			t.Fatal(m.Err)
		}
		if index == 1 {
			if _, found := m.Metrics["midpoint_response_per_net_notional"]; found {
				t.Fatal("zero-net response invented")
			}
		}
		if index == 2 {
			if _, found := m.Metrics["midpoint_response_per_net_notional"]; !found {
				t.Fatal("next run was lost")
			}
		}
	}
}
