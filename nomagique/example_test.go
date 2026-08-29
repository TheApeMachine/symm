package nomagique

import (
	"fmt"

	"github.com/theapemachine/symm/nomagique/calculus"
	"github.com/theapemachine/symm/nomagique/types"
)

func ExampleStep() {
	output := types.Frame{}
	output.Put(calculus.SymbolLeft, 3)
	output.Put(calculus.SymbolRight, 4)
	Step(calculus.Sum, &output)

	if output.Err != nil {
		panic(output.Err)
	}

	fmt.Println(output.MustGet(calculus.SymbolResult))
	// Output: 7
}

func ExampleKeyedStreams() {
	totalSymbol := types.MustIntern("example/total")
	deltaSymbol := types.MustIntern("example/delta")
	accumulate := func(input *types.Frame) {
		total, _ := input.Get(totalSymbol)
		input.Put(totalSymbol, total+input.MustGet(deltaSymbol))
	}
	streams := NewKeyedStreams[string](accumulate, nil)
	input := types.Frame{}
	input.Put(deltaSymbol, 2)
	_ = streams.Step("A", input)
	output := streams.Step("A", input)

	fmt.Println(output.MustGet(totalSymbol))
	// Output: 4
}
