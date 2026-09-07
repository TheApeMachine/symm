package leadlag

import (
	"github.com/theapemachine/symm/nomagique/algo"
	"github.com/theapemachine/symm/nomagique/core"
	nmcorrelation "github.com/theapemachine/symm/nomagique/correlation"
	"github.com/theapemachine/symm/nomagique/transport"
	"math"
	"testing"
)

func TestPrimitiveLagSearchAndHistory(t *testing.T) {
	built := newPipeline()
	search := nmcorrelation.NewLeadLag(algo.NewHayashiYoshida())
	left, right := nmcorrelation.NewPath(), nmcorrelation.NewPath()
	observations := 0
	for index := 0; index < 12; index++ {
		at := int64(index) * 1_000_000_000
		l, err := transport.Evaluate[map[string]core.Primitive](left, core.Record(map[string]any{"at": at, "value": 100 * math.Exp(.005*float64(index)+.02*math.Sin(float64(index)))}))
		if err != nil {
			t.Fatal(err)
		}
		r, err := transport.Evaluate[map[string]core.Primitive](right, core.Record(map[string]any{"at": at, "value": 50 * math.Exp(.004*float64(index)+.02*math.Sin(float64(index-1)))}))
		if err != nil {
			t.Fatal(err)
		}
		pair, err := transport.Evaluate[map[string]core.Primitive](search, core.From(map[string]core.Primitive{"left": l["observations"], "right": r["observations"]}))
		if err != nil {
			t.Fatal(index, err)
		}
		defined, err := core.Field[bool](pair, "defined")
		if err != nil {
			t.Fatal(err)
		}
		if !defined {
			continue
		}
		observations++
		out, err := transport.Evaluate[map[string]core.Primitive](built.progress, core.Record(map[string]any{"at": at, "pair": pair}))
		if err != nil {
			t.Fatal(index, err)
		}
		count, err := core.Field[float64](out, "correlation_history", "count")
		if err != nil {
			t.Fatal(err)
		}
		if count > float64(observations) || count < 1 {
			t.Fatalf("invalid retained history %v after %d observations", count, observations)
		}
		m := built.projection.Project(out)
		if m.Err != nil {
			t.Fatal(index, m.Err)
		}
		if len(m.Metrics) == 0 {
			t.Fatal("empty projection")
		}
	}
	if observations < 3 {
		t.Fatalf("only %d defined searches", observations)
	}
}
