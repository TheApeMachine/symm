package correlation

import (
	"math"
	"testing"

	nmcorrelation "github.com/theapemachine/symm/nomagique/correlation"
	"github.com/theapemachine/symm/nomagique/equation"
	"github.com/theapemachine/symm/nomagique/transport"
)

func TestPipelinePair(t *testing.T) {
	built, left, right := newPipeline(), nmcorrelation.NewPath(), nmcorrelation.NewPath()
	var peer nmcorrelation.PathReading
	for index, price := range []float64{100, 101, 102, 103, 104, 105, 106, 107, 108, 109, 110, 111} {
		var err error
		peer, err = transport.Evaluate(right, transport.Values(equation.Price{Value: price, At: int64(index+1) * 1e9}))
		if err != nil {
			t.Fatal(err)
		}
	}
	var finalSNR bool
	for index, price := range []float64{200, 202, 201, 205, 203, 208, 204, 211, 206, 214, 208, 217} {
		focal, err := transport.Evaluate(left, transport.Values(equation.Price{Value: price, At: int64(index+1) * 1e9}))
		if err != nil {
			t.Fatal(err)
		}
		pair, err := built.pair(focal.Observations, peer.Observations)
		if err != nil {
			t.Fatal(err)
		}
		if pair.dependence.Support < 2 {
			continue
		}
		cohort, err := built.fold([]nmcorrelation.Peer{{
			Correlation: pair.dependence.Correlation,
			Support:     pair.dependence.Support,
			PeerEnergy:  pair.dependence.RightEnergyRate,
		}})
		if err != nil {
			t.Fatal(err)
		}
		progress, err := built.advance(pair, cohort, int64(index+1)*1e9)
		if err != nil {
			t.Fatal(err)
		}
		m := built.projection.Project(progress)
		if m.Err != nil {
			t.Fatal(m.Err)
		}
		if math.IsNaN(m.Metrics["signed_correlation"].Raw) {
			t.Fatal("lost signed evidence")
		}
		if m.Metrics["cohort_peer_count"].Raw != 1 {
			t.Fatal("cohort leaked across runs")
		}
		finalSNR = m.SNRDefined
	}
	if !finalSNR {
		t.Fatal("settled Fisher history not projected")
	}
}
