package toxicity

import (
	"testing"

	"github.com/theapemachine/symm/nomagique/transport"
)

func TestTradeGraphNext(t *testing.T) {
	graph, p := newTradeGraph(), tradeProjection()
	for index, fraction := range []float64{.3, .5, .9, 1.2} {
		fields, err := transport.Evaluate(graph, transport.Values(TradeInput{
			BracketQty: fraction * 10, MatchedBidQty: fraction * 10,
			TouchFillBidQty: fraction * 10, TouchFillBidFrac: fraction,
			HasRate: index > 0, TouchFillBidRate: fraction * 10,
			BidSupported: index >= 2,
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

func TestLevel3GraphNext(t *testing.T) {
	graph, p := newLevel3Graph(), level3Projection()
	input := Level3Input{CurBid: 99, CurAsk: 101, PrevBid: 99, PrevAsk: 101, CurBidQty: 10, CurAskQty: 12, PrevBidQty: 10, PrevAskQty: 12, UnfilledBid: 10, UnfilledAsk: 12}
	for index := 0; index < 2; index++ {
		if index == 1 {
			input.WithFracBid = .6
		}
		fields, err := transport.Evaluate(graph, transport.Values(input))
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
