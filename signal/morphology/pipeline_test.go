package morphology

import (
	"context"
	"errors"
	"math"
	"testing"

	"github.com/krakenfx/api-go/v2/pkg/spot"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/nomagique/core"
)

type pipelineWrapper struct {
	signal *Signal
	err    error
}

func newPipeline() *pipelineWrapper {
	ctx := context.Background()
	return &pipelineWrapper{signal: NewSignal(ctx, broker.NewBook(ctx, spot.NewNormalizer()))}
}

func (p *pipelineWrapper) Error() error {
	return p.err
}

func observe(t *testing.T, p *pipelineWrapper, key string, bids, asks []float64) []float64 {
	t.Helper()
	res, err := p.signal.Calculate(key, bids, asks)
	p.err = err
	return res
}

func mirrored(outerBid, outerAsk float64) ([]float64, []float64) {
	return []float64{10, 14, 10, .1, outerBid, 1 / outerBid}, []float64{10, 14, 14, 1.0 / 14, outerAsk, 1 / outerAsk}
}

func assertValues(t *testing.T, got, want []float64) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for index := range want {
		if math.IsNaN(got[index]) || math.Abs(got[index]-want[index]) > 1e-11 {
			t.Fatalf("metric %d: got %.17g, want %.17g", index, got[index], want[index])
		}
	}
}

func TestPipelineMirroredShapes(t *testing.T) {
	pipeline := newPipeline()
	bids, asks := mirrored(6, 18)
	got := observe(t, pipeline, "1/BTC/USD", bids, asks)
	assertValues(t, got, []float64{0, 0, .5, .5, math.Log(2), math.Log(2)})
	if err := pipeline.Error(); err != nil {
		t.Fatal(err)
	}
}

func TestPipelineWholeBookChangeNotChangeOfDistance(t *testing.T) {
	pipeline := newPipeline()
	bids, asks := mirrored(6, 18)
	observe(t, pipeline, "1/BTC/USD", bids, asks)
	bids, asks = mirrored(2, 22)
	got := observe(t, pipeline, "1/BTC/USD", bids, asks)
	assertValues(t, got, []float64{0, 0, .5, .5, math.Log(2), math.Log(2), .5})
	got = observe(t, pipeline, "1/BTC/USD", bids, asks)
	assertValues(t, got, []float64{0, 0, .5, .5, math.Log(2), math.Log(2), 0})
	if err := pipeline.Error(); err != nil {
		t.Fatal(err)
	}
}

func TestPipelineSymbolAndEpochIsolation(t *testing.T) {
	pipeline := newPipeline()
	bids, asks := mirrored(6, 18)
	first := observe(t, pipeline, "1/A/USD", bids, asks)
	otherBids, otherAsks := mirrored(2, 22)
	other := observe(t, pipeline, "1/B/USD", otherBids, otherAsks)
	epoch := observe(t, pipeline, "2/A/USD", otherBids, otherAsks)
	if len(first) != 6 || len(other) != 6 || len(epoch) != 6 {
		t.Fatalf("first observations fabricated a prior shape: %v %v %v", first, other, epoch)
	}
	got := observe(t, pipeline, "1/A/USD", bids, asks)
	if len(got) != 7 || got[6] != 0 {
		t.Fatalf("another key changed A's retained shape: %v", got)
	}
	got = observe(t, pipeline, "1/B/USD", otherBids, otherAsks)
	if len(got) != 7 || got[6] != 0 {
		t.Fatalf("another key changed B's retained shape: %v", got)
	}
	if err := pipeline.Error(); err != nil {
		t.Fatal(err)
	}
}

func TestPipelineSignedSideSwap(t *testing.T) {
	pipeline := newPipeline()
	bids := []float64{10, 14, 10, 1.0 / 10, 6, 3.0 / 6}
	asks := []float64{10, 14, 14, 3.0 / 14, 18, 1.0 / 18}
	first := observe(t, pipeline, "1/A", bids, asks)
	entropy := -.25*math.Log(.25) - .75*math.Log(.75)
	assertValues(t, first, []float64{.5, .5, .625, .625, entropy, entropy})
	bids = []float64{10, 14, 10, 3.0 / 10, 6, 1.0 / 6}
	asks = []float64{10, 14, 14, 1.0 / 14, 18, 3.0 / 18}
	got := observe(t, pipeline, "1/A", bids, asks)
	assertValues(t, got, []float64{.5, .5, .625, .625, entropy, entropy, .5})
}

func TestPipelinePriceScaleInvariance(t *testing.T) {
	pipeline := newPipeline()
	bids, asks := mirrored(6, 18)
	first := observe(t, pipeline, "1/A", bids, asks)
	for index := range bids {
		if index < 2 || index%2 == 0 {
			bids[index] *= 17
		}
	}
	for index := range asks {
		if index < 2 || index%2 == 0 {
			asks[index] *= 17
		}
	}
	got := observe(t, pipeline, "1/A", bids, asks)
	assertValues(t, got, append(first, 0))
}

func TestPipelineZeroAndCrossedSpread(t *testing.T) {
	for _, ask := range []float64{10, 9} {
		pipeline := newPipeline()
		got := observe(t, pipeline, "1/A", []float64{10, ask, 10, 1}, []float64{10, ask, ask, 1})
		if len(got) != 0 || !errors.Is(pipeline.Error(), core.ErrDomain) {
			t.Fatalf("invalid spread emitted %v, error %v", got, pipeline.Error())
		}
	}
}

func TestPipelineEverySuppliedLevel(t *testing.T) {
	pipeline := newPipeline()
	bids := []float64{1000, 1002}
	asks := []float64{1000, 1002}
	for index := range 150 {
		bid := 1000 - float64(index)*2
		ask := 1002 + float64(index)*2
		bids = append(bids, bid, 1/bid)
		asks = append(asks, ask, 1/ask)
	}
	first := observe(t, pipeline, "1/A", bids, asks)
	if len(first) != 6 {
		t.Fatalf("first=%v err=%v", first, pipeline.Error())
	}
	// Only the deepest ask level moves; first 149 levels remain identical.
	asks[len(asks)-2] += 2
	asks[len(asks)-1] = 1 / asks[len(asks)-2]
	got := observe(t, pipeline, "1/A", bids, asks)
	if len(got) != 7 {
		t.Fatalf("got=%v err=%v", got, pipeline.Error())
	}
	if math.Abs(got[0]-1.0/150) > 1e-11 || math.Abs(got[6]-1.0/300) > 1e-11 {
		t.Fatalf("deep level lost: %v", got)
	}
}

func TestPipelineEmptySideDoesNotAdvanceHistory(t *testing.T) {
	pipeline := newPipeline()
	bids, asks := mirrored(6, 18)
	observe(t, pipeline, "1/A", bids, asks)
	empty := []float64{10, 14, 10, 0, 6, 0}
	if got := observe(t, pipeline, "1/A", empty, asks); len(got) != 0 || pipeline.Error() != nil {
		t.Fatalf("empty side: output=%v err=%v", got, pipeline.Error())
	}
	got := observe(t, pipeline, "1/A", bids, asks)
	if len(got) != 7 || got[6] != 0 {
		t.Fatalf("empty side changed history: %v", got)
	}
}
