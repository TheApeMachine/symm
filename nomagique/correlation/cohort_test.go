package correlation_test

import (
	"github.com/theapemachine/symm/nomagique/calculus"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/correlation"
	"github.com/theapemachine/symm/nomagique/tests"
	"github.com/theapemachine/symm/nomagique/transport"
	"math"
	"testing"
)

func TestCohortNext(t *testing.T) {
	node := correlation.NewCohort(calculus.NewAtanh(transport.NewIO(core.From(0.0))))
	source := tests.Values(
		tests.Record(map[string]any{"correlation": .4, "support": 3.0, "peer_energy": 2.0}),
		tests.Record(map[string]any{"correlation": -.2, "support": 2.0, "peer_energy": 4.0}),
		tests.Record(map[string]any{"correlation": .9, "support": 1.0}))
	out := tests.Drain(t, node, source)
	tests.Sound(t, node)
	f := tests.Fields(t, out[0])
	z1, z2 := math.Atanh(.4), math.Atanh(-.2)
	mean := (3*z1 + 2*z2) / 5
	for key, want := range map[string]float64{
		"peers_seen": 3, "peers": 2, "rejected_peers": 1, "total_support": 5,
		"effective_peers": 25.0 / 13, "signed_correlation": .16, "absolute_correlation": .32,
		"peer_energy_rate": 2.8, "dispersion": math.Sqrt((3*z1*z1+2*z2*z2)/5 - mean*mean),
	} {
		tests.EqualNumber(t, tests.Number(t, f, key), want)
	}
	empty := tests.Drain(t, node, tests.Values[core.Primitive]())
	tests.Sound(t, node)
	f = tests.Fields(t, empty[0])
	if core.To[bool](f["defined"]) {
		t.Fatal("empty cohort reused old evidence")
	}
	tests.EqualNumber(t, tests.Number(t, f, "signed_correlation"), math.NaN())
}
