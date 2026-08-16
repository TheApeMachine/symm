package algo

import (
	"fmt"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique"
	"github.com/theapemachine/symm/nomagique/calculus"
	"github.com/theapemachine/symm/nomagique/logic"
	"github.com/theapemachine/symm/nomagique/probability"
	"github.com/theapemachine/symm/nomagique/statistic"
	"github.com/theapemachine/symm/nomagique/temporal"
	"github.com/theapemachine/symm/nomagique/types"
)

func (ignition *Ignition) evidence(ready bool, values ...float64) (float64, error) {
	score := 0.0
	if ready {
		params := types.NewMap[string, types.Value[float64]]()
		for index, value := range values {
			params.Put(fmt.Sprintf("sample/%d", index), types.NewValue(value))
		}
		input := types.NewInput(types.NewValue(params))
		primitive := probability.NewGeomean(input)
		output, err := ignition.execute("geometric mean", input, primitive)
		if err != nil {
			return 0, err
		}
		score = ignitionNumber(output, "result")
	}

	params := ignitionParams(
		ignitionParam("condition", ignitionBool(ready)),
		ignitionParam("value", score),
	)
	input := types.NewInput(types.NewValue(params))
	primitive := logic.NewGate(input)
	output, err := ignition.execute("evidence gate", input, primitive)
	if err != nil {
		return 0, err
	}
	return ignitionNumber(output, "result"), nil
}

func (ignition *Ignition) sum(left float64, right float64) (float64, error) {
	params := ignitionParams(
		ignitionParam("left", left),
		ignitionParam("right", right),
	)
	input := types.NewInput(types.NewValue(params))
	return ignition.result("sum", input, calculus.NewSum(input), "result")
}

func (ignition *Ignition) difference(left float64, right float64) (float64, error) {
	params := ignitionParams(
		ignitionParam("left", left),
		ignitionParam("right", right),
	)
	input := types.NewInput(types.NewValue(params))
	return ignition.result("difference", input, calculus.NewDifference(input), "result")
}

func (ignition *Ignition) positive(value float64) (float64, error) {
	params := ignitionParams(ignitionParam("value", value))
	input := types.NewInput(types.NewValue(params))
	return ignition.result("positive", input, calculus.NewPositive(input), "result")
}

func (ignition *Ignition) product(left float64, right float64) (float64, error) {
	params := ignitionParams(
		ignitionParam("left", left),
		ignitionParam("right", right),
	)
	input := types.NewInput(types.NewValue(params))
	return ignition.result("product", input, calculus.NewProduct(input), "result")
}

func (ignition *Ignition) logRatio(current float64, previous float64) (float64, error) {
	params := ignitionParams(
		ignitionParam("current", current),
		ignitionParam("previous", previous),
	)
	input := types.NewInput(types.NewValue(params))
	return ignition.result("log ratio", input, calculus.NewLogRatio(input), "result")
}

func (ignition *Ignition) squash(value float64, scale float64) (float64, error) {
	params := ignitionParams(
		ignitionParam("value", value),
		ignitionParam("scale", scale),
	)
	input := types.NewInput(types.NewValue(params))
	return ignition.result("squash", input, calculus.NewSquash(input), "result")
}

func (ignition *Ignition) inverse(value float64, scale float64) (float64, error) {
	params := ignitionParams(
		ignitionParam("value", value),
		ignitionParam("scale", scale),
	)
	input := types.NewInput(types.NewValue(params))
	return ignition.result("inverse", input, calculus.NewInverse(input), "result")
}

func (ignition *Ignition) ratio(value float64, baseline float64, ready bool) (float64, error) {
	params := ignitionParams(
		ignitionParam("value", value),
		ignitionParam("baseline", baseline),
		ignitionParam("ready", ignitionBool(ready)),
	)
	input := types.NewInput(types.NewValue(params))
	return ignition.result("ratio", input, calculus.NewRatio(input), "result")
}

func (ignition *Ignition) rate(count float64, duration float64) (float64, error) {
	params := ignitionParams(
		ignitionParam("count", count),
		ignitionParam("duration", duration),
	)
	input := types.NewInput(types.NewValue(params))
	return ignition.result("rate", input, calculus.NewRate(input), "rate")
}

func (ignition *Ignition) duration(
	currentSec float64,
	currentNsec float64,
	previousSec float64,
	previousNsec float64,
) (float64, error) {
	params := ignitionParams(
		ignitionParam("current_sec", currentSec),
		ignitionParam("current_nsec", currentNsec),
		ignitionParam("previous_sec", previousSec),
		ignitionParam("previous_nsec", previousNsec),
	)
	input := types.NewInput(types.NewValue(params))
	return ignition.result("duration", input, temporal.NewDuration(input), "delta")
}

func (ignition *Ignition) maximum(values ...float64) (float64, error) {
	params := types.NewMap[string, types.Value[float64]]()
	for index, value := range values {
		params.Put(fmt.Sprintf("sample/%d", index), types.NewValue(value))
	}
	input := types.NewInput(types.NewValue(params))
	return ignition.result("maximum", input, statistic.NewMaximum(input), "result")
}

func (ignition *Ignition) result(
	name string,
	input types.Input[ignitionMap],
	primitive types.IO[ignitionMap],
	key string,
) (float64, error) {
	output, err := ignition.execute(name, input, primitive)
	if err != nil {
		return 0, err
	}
	value, found := output.Get(key)
	if !found {
		return 0, ignitionError(name + " did not produce " + key)
	}
	return value.Read(), nil
}

func (ignition *Ignition) execute(
	name string,
	input types.Input[ignitionMap],
	primitive types.IO[ignitionMap],
) (ignitionMap, error) {
	output := nomagique.Number(input, primitive)
	if output.Error() != "" {
		return ignitionMap{}, errnie.Error(errnie.Err(
			errnie.Validation,
			"ignition: "+name,
			fmt.Errorf("%s", output.Error()),
		))
	}
	return output.Project().Read(), nil
}
