package toxicity

import (
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/transport"
	"testing"
)

func TestTradeGraphPrimitiveFractions(t *testing.T) {
	graph, p := newTradeGraph(), tradeProjection()
	for index, fraction := range []float64{.3, .5, .9, 1.2} {
		fields, err := transport.Evaluate[map[string]core.Primitive](graph, core.Record(map[string]any{
			"bracketQty": fraction * 10, "matchedBidQty": fraction * 10, "matchedAskQty": 0.0, "touchFillBidQty": fraction * 10, "touchFillAskQty": 0.0,
			"touchFillBidFrac": fraction, "touchFillAskFrac": 0.0, "hasRate": index > 0, "touchFillBidRate": fraction * 10, "touchFillAskRate": 0.0,
			"bidSupported": index >= 2, "askSupported": false,
		}))
		if err != nil {
			t.Fatal(err)
		}
		m := p.Project(fields)
		if m.Err != nil {
			t.Fatal(m.Err)
		}
		if index == 0 && m.SNRDefined {
			t.Fatal("cold SNR")
		}
		if index == 1 && m.Metrics["fill_fraction_baseline:bid"].Raw != .3 {
			t.Fatal("causal baseline lost")
		}
		if index == 3 && !m.SNRDefined {
			t.Fatal("estimated noise lost")
		}
	}
}

func TestLevel3GraphPrimitiveDirectFacts(t *testing.T) {
	graph, p := newLevel3Graph(), level3Projection()
	values := map[string]any{"curBid": 99.0, "curAsk": 101.0, "prevBid": 99.0, "prevAsk": 101.0, "curBidQty": 10.0, "curAskQty": 12.0, "prevBidQty": 10.0, "prevAskQty": 12.0, "unfilledBid": 10.0, "unfilledAsk": 12.0, "hasRate": false}
	for _, name := range []string{"logChangeBid", "logChangeAsk", "retreatedBid", "retreatedAsk", "withdrawnBid", "withdrawnAsk", "replenishedBid", "replenishedAsk", "retreatFracBid", "retreatFracAsk", "withFracBid", "withFracAsk", "repFracBid", "repFracAsk"} {
		values[name] = 0.0
	}
	for index := 0; index < 2; index++ {
		if index == 1 {
			values["withFracBid"] = .6
		}
		fields, err := transport.Evaluate[map[string]core.Primitive](graph, core.Record(values))
		if err != nil {
			t.Fatal(err)
		}
		m := p.Project(fields)
		if m.Err != nil {
			t.Fatal(m.Err)
		}
		if m.Maturity != 1 || m.SNRDefined {
			t.Fatal("direct touch evidence reclassified")
		}
		if index == 1 && m.Metrics["withdrawal_fraction_baseline:bid"].Raw != .3 {
			t.Fatal("inclusive mean mapping changed")
		}
	}
}
