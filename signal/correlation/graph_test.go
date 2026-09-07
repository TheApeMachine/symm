package correlation

import (
	"github.com/theapemachine/symm/nomagique/core"
	nmcorrelation "github.com/theapemachine/symm/nomagique/correlation"
	"github.com/theapemachine/symm/nomagique/transport"
	"math"
	"testing"
)

func TestCorrelationPrimitivePipeline(t *testing.T) {
	built, left, right := newPipeline(), nmcorrelation.NewPath(), nmcorrelation.NewPath()
	var focal, peer map[string]core.Primitive
	for index, price := range []float64{100, 101, 102, 103, 104, 105, 106, 107, 108, 109, 110, 111} {
		var err error
		peer, err = transport.Evaluate[map[string]core.Primitive](right, core.Record(map[string]any{"value": price, "at": int64(index+1) * 1e9}))
		if err != nil {
			t.Fatal(err)
		}
	}
	var finalSNR bool
	for index, price := range []float64{200, 202, 201, 205, 203, 208, 204, 211, 206, 214, 208, 217} {
		var err error
		focal, err = transport.Evaluate[map[string]core.Primitive](left, core.Record(map[string]any{"value": price, "at": int64(index+1) * 1e9}))
		if err != nil {
			t.Fatal(err)
		}
		pair, err := transport.Evaluate[map[string]core.Primitive](built.pairwise, core.From(map[string]core.Primitive{"left": focal["observations"], "right": peer["observations"]}))
		if err != nil {
			t.Fatal(err)
		}
		support, err := core.Field[float64](pair, "support")
		if err != nil {
			t.Fatal(err)
		}
		if support < 2 {
			continue
		}
		record := core.From(map[string]core.Primitive{"correlation": pair["correlation"], "support": pair["support"], "peer_energy": pair["right_energy_rate"]})
		cohort, err := transport.Evaluate[map[string]core.Primitive](built.cohort, core.From([]core.Primitive{record}))
		if err != nil {
			t.Fatal(err)
		}
		result, err := transport.Evaluate[map[string]core.Primitive](built.progress, core.From(map[string]core.Primitive{"cohort": core.From(cohort), "pair": core.From(pair), "at": core.From(int64(index+1) * 1e9)}))
		if err != nil {
			t.Fatal(err)
		}
		m := built.projection.Project(result)
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
