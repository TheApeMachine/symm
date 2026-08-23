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
	_, output, err := Step(calculus.Sum, types.Frame{}, input)

	if err != nil {
		panic(err)
	}

	fmt.Println(output.MustGet(calculus.SymbolResult))
	// Output: 7
}

func ExampleKeyedStreams() {
	totalSymbol := types.MustIntern("example/total")
	deltaSymbol := types.MustIntern("example/delta")
	accumulate := func(
		state types.Frame,
		input types.Frame,
	) (types.Frame, types.Frame, error) {
		total, _ := state.Get(totalSymbol)
		nextState := state
		nextState.Put(totalSymbol, total+input.MustGet(deltaSymbol))

		return nextState, nextState, nil
	}
	streams := NewKeyedStreams[string](accumulate, nil)
	input := types.Frame{}
	input.Put(deltaSymbol, 2)
	_, _ = streams.Step("A", input)
	output, _ := streams.Step("A", input)

	fmt.Println(output.MustGet(totalSymbol))
	// Output: 4
}
