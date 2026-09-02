package main

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	nmtypes "github.com/theapemachine/symm/nomagique/types"

	"github.com/theapemachine/symm/signal/correlation"
	"github.com/theapemachine/symm/signal/cvd"
	"github.com/theapemachine/symm/signal/depthflow"
	"github.com/theapemachine/symm/signal/derivatives"
	"github.com/theapemachine/symm/signal/hawkes"
	"github.com/theapemachine/symm/signal/leadlag"
	"github.com/theapemachine/symm/signal/liquidity"
	"github.com/theapemachine/symm/signal/morphology"
	"github.com/theapemachine/symm/signal/pumpdump"
	"github.com/theapemachine/symm/signal/sentiment"
	"github.com/theapemachine/symm/signal/toxicity"

	_ "github.com/theapemachine/symm/logic/resonance"
	_ "github.com/theapemachine/symm/nomagique/adaptive"
	_ "github.com/theapemachine/symm/nomagique/causal"
	_ "github.com/theapemachine/symm/nomagique/correlation"
	_ "github.com/theapemachine/symm/nomagique/equation"
	_ "github.com/theapemachine/symm/nomagique/learning"
	_ "github.com/theapemachine/symm/nomagique/probability"
	_ "github.com/theapemachine/symm/nomagique/recurrence"
	_ "github.com/theapemachine/symm/nomagique/statistic/hawkes"
	_ "github.com/theapemachine/symm/nomagique/transport"
	_ "github.com/theapemachine/symm/strategy"
	_ "github.com/theapemachine/symm/types"
)

func main() {
	ctx := context.Background()
	fmt.Printf("after package init:        %d\n", nmtypes.RegisteredSymbols())

	correlation.NewSignal(ctx)
	leadlag.NewSignal(ctx)
	liquidity.NewSignal(ctx)
	sentiment.NewSignal(ctx)
	pumpdump.NewSignal(ctx, func(symbol string) (*decimal.Decimal, *decimal.Decimal) { return nil, nil })
	hawkes.NewSignal(ctx)
	depthflow.NewSignal(ctx)
	morphology.NewSignal(ctx)
	derivatives.NewSignal(ctx)
	toxicity.NewSignal(ctx)
	cvd.NewSignal(ctx, func(symbol string) (*decimal.Decimal, *decimal.Decimal) { return nil, nil })

	fmt.Printf("after constructing signals: %d\n", nmtypes.RegisteredSymbols())

	// Enumerate every distinct series prefix that owns a sample ring. Each can
	// lazily intern up to MaxSamples slots as its elastic window grows.
	series := map[string]bool{}
	for index := range nmtypes.RegisteredSymbols() {
		name, found := nmtypes.SymbolName(nmtypes.Symbol(index))

		if !found {
			panic(fmt.Sprintf("registered symbol %d has no name", index))
		}

		for _, suffix := range []string{"/capacity", "/count", "/head", "/ready"} {
			if strings.HasSuffix(name, suffix) {
				series[strings.TrimSuffix(name, suffix)] = true
			}
		}
	}

	keys := make([]string, 0, len(series))
	for key := range series {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	fmt.Printf("\ndistinct sample-ring series: %d\n", len(keys))
	fmt.Printf("worst-case sample slots:     %d x %d = %d\n",
		len(keys), nmtypes.MaxSamples, len(keys)*nmtypes.MaxSamples)
	fmt.Printf("projected total demand:      %d\n",
		nmtypes.RegisteredSymbols()+len(keys)*nmtypes.MaxSamples)
	fmt.Printf("MaxSlots:                    %d\n\n", nmtypes.MaxSlots)
	for _, key := range keys {
		fmt.Printf("  %s\n", key)
	}
}
