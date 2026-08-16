package nomagique_test

import (
	"fmt"

	"github.com/theapemachine/symm/nomagique"
	"github.com/theapemachine/symm/nomagique/calculus"
)

func ExampleStep() {
	input := nomagique.Frame{}
	input.Put(calculus.SymbolLeft, 3)
	input.Put(calculus.SymbolRight, 4)
	_, output, err := nomagique.Step(calculus.Sum, nomagique.Frame{}, input)

	if err != nil {
		panic(err)
	}

	fmt.Println(output.MustGet(calculus.SymbolResult))
	// Output: 7
}

func ExampleKeyedStreams() {
	totalSymbol := nomagique.MustIntern("example/total")
	deltaSymbol := nomagique.MustIntern("example/delta")
	accumulate := func(
		state nomagique.Frame,
		input nomagique.Frame,
	) (nomagique.Frame, nomagique.Frame, error) {
		total, _ := state.Get(totalSymbol)
		nextState := state
		nextState.Put(totalSymbol, total+input.MustGet(deltaSymbol))

		return nextState, nextState, nil
	}
	streams := nomagique.NewKeyedStreams[string](accumulate, nil)
	input := nomagique.Frame{}
	input.Put(deltaSymbol, 2)
	_, _ = streams.Step("A", input)
	output, _ := streams.Step("A", input)

	fmt.Println(output.MustGet(totalSymbol))
	// Output: 4
}
