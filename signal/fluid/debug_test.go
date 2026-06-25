package fluid

import (
	"context"
	"fmt"
	"testing"

	"github.com/theapemachine/datura"
	"github.com/theapemachine/datura/dmt"
)

func TestDebugFluidHistory(t *testing.T) {
	signal := NewSignal(context.Background(), newTestPool(t), dmt.NewTree(""))
	symbol := "ETH/EUR"
	warmupStableTicker(signal, symbol, 60)
	result := measureTickerFrame(signal, symbol, 1060, 100, 99.99, 100.01)

	if result == nil {
		t.Fatal("nil result")
	}

	fmt.Printf("insert prefix=%q\n", string(result.Prefix()))

	for _, key := range []string{"laminar", "turbulent", "inertial", "viscous", "viscosity"} {
		fmt.Printf("%s=%v\n", key, outputScore(result, key))
	}

	query := datura.Acquire("fluid", datura.APPJSON)
	query.WithRole("measurement")
	query.WithScope(symbol)
	_ = query.SetOrigin("fluid")

	fmt.Printf("seek prefix=%q\n", string(query.Prefix("role", "scope", "origin")))

	count := 0

	for prior := range signal.tree.Seek(query.Prefix("role", "scope", "origin")) {
		count++
		fmt.Printf("prior spread=%v displacement=%v\n",
			datura.Peek[float64](prior, "spread"),
			datura.Peek[float64](prior, "displacement"),
		)
	}

	fmt.Printf("history count=%d\n", count)
}
