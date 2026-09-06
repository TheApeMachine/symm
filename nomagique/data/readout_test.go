package data_test

import (
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/store"
	"github.com/theapemachine/symm/nomagique/tests"
	"github.com/theapemachine/symm/nomagique/transport"
	"testing"
)

func TestReadoutNext(t *testing.T) {
	node := data.NewReadout(data.NewAuthority(), transport.NewPipe(store.NewGet("supports"), transport.NewSpread[float64]()),
		transport.NewPipe(store.NewGet("contradictions"), transport.NewSpread[float64]()), store.NewGet("credibility"), store.NewGet("coordinate"))
	for _, c := range []struct {
		maturity, snr, cred      float64
		supports, contradictions []float64
		defined, coordinate      bool
	}{
		{0.8, 5, 1, []float64{0.1}, []float64{0.5}, true, false},
		{0.8, 5, 0.2, []float64{}, []float64{}, true, false},
		{0.8, 5, 1, []float64{}, []float64{}, false, false},
		{0.8, 5, 1, []float64{}, []float64{}, false, true},
	} {
		input := tests.Record(map[string]any{"maturity": c.maturity, "snr": c.snr, "estimated": true, "snr_defined": true,
			"raw": 10.0, "credibility": c.cred, "supports": c.supports, "contradictions": c.contradictions, "defined": c.defined, "coordinate": c.coordinate})
		fields := tests.Fields(t, tests.Drain(t, node, tests.Values(input))[0])
		tests.Sound(t, node)
		a := c.maturity * c.snr / (1 + c.snr) * c.cred
		s, op := 0.0, 0.0
		for _, v := range c.supports {
			s += v
		}
		for _, v := range c.contradictions {
			op += v
		}
		a *= (1 + s) / (1 + 2*op)
		if a > 1 {
			a = 1
		}
		if !c.defined {
			a = 0
		}
		tests.EqualNumber(t, tests.Number(t, fields, "authority"), a)
		value := 10 * a
		if c.coordinate {
			value = 10
		}
		tests.EqualNumber(t, tests.Number(t, fields, "value"), value)
	}
}
