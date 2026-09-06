package equation_test

import (
	"errors"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/equation"
	"github.com/theapemachine/symm/nomagique/store"
	"github.com/theapemachine/symm/nomagique/tests"
	"github.com/theapemachine/symm/nomagique/transport"
	"testing"
)

func TestNewRenewalRate(t *testing.T) {
	target := store.NewRetained(core.From(4.0))
	tests.CheckRenewalRate(t, equation.NewRenewalRate(transport.NewApply(target, nil)))
	tests.CheckRenewalMissing(t, equation.NewRenewalRate(store.NewConstant(core.From(4.0))))
	tests.Drain(t, target, tests.Values(0.0))
	node := equation.NewRenewalRate(transport.NewApply(target, nil))
	out := tests.Drain(t, node, tests.Values(tests.Record(map[string]any{"increment": 2.0, "sample": 100.0, "at": int64(0)})))
	if len(out) != 0 || !errors.Is(node.Error(), core.ErrDomain) {
		t.Fatal("invalid target accepted")
	}
}

func BenchmarkNewRenewalRate(b *testing.B) {
	rate := equation.NewRenewalRate(store.NewConstant(core.From(4.0)))
	var timestamp int64
	b.ReportAllocs()

	for iteration := 0; iteration < b.N; iteration++ {
		input := transport.NewIO(tests.Record(map[string]any{
			"increment": 2.0, "sample": 100.0, "at": timestamp,
		}))

		if rate.Next(input) == nil || rate.Next(input) != nil {
			b.Fatal("expected one renewal record")
		}

		timestamp += 1e9
	}

	if err := rate.Error(); err != nil {
		b.Fatal(err)
	}
}
