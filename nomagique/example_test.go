package nomagique

import (
	"fmt"

	"github.com/theapemachine/symm/nomagique/calculus"
	"github.com/theapemachine/symm/nomagique/types"
)

func ExampleStep() {
	input := types.Frame{}
	input.Put(calculus.SymbolLeft, 3)
	input.Put(calculus.SymbolRight, 4)
	output := Step(calculus.Sum, input)

	if output.Err != nil {
		panic(output.Err)
	}

	fmt.Println(output.MustGet(calculus.SymbolResult))
	// Output: 7
}

func ExampleKeyedStreams() {
	totalSymbol := types.MustIntern("example/total")
	deltaSymbol := types.MustIntern("example/delta")
	accumulate := func(input types.Frame) types.Frame {
		total, _ := input.Get(totalSymbol)
		input.Put(totalSymbol, total+input.MustGet(deltaSymbol))

		return input
	}
	streams := NewKeyedStreams[string](accumulate, nil)
	input := types.Frame{}
	input.Put(deltaSymbol, 2)
	_ = streams.Step("A", input)
	output := streams.Step("A", input)

	fmt.Println(output.MustGet(totalSymbol))
	// Output: 4
}
