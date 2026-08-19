package nomagique

type Number func(input Frame) (Frame, error)

/*
Number composes a pipeline of primitives into a state-carrying callable. Its
output feeds the next primitive's input, and the composed state persists across
calls so windows and baselines accumulate history. It is the ergonomic entry
point for building a reusable numeric unit:

	number := nomagique.Number(temporal.A, statistic.B, probability.C)
	output := number(input)

State is carried inside the returned callable and guarded against concurrent
use. Callers that manage their own explicit Frame state should use Pipe, which
remains the stateless, by-value composition path.
*/
func NewNumber(primitives ...Primitive) Number {
	pipeline := Pipe(primitives...)

	var (
		state Frame
	)

	return func(input Frame) (Frame, error) {
		nextState, output, err := Step(pipeline, state, input)

		if err != nil {
			return Frame{}, err
		}

		state = nextState

		return output, nil
	}
}
