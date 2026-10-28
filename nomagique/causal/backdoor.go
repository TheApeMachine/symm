package causal

import "github.com/theapemachine/errnie"

/*
BackdoorConfig describes a linear backdoor-adjusted treatment estimate.
*/
type BackdoorConfig struct {
	Target    int
	Treatment int
	Controls  []int
}

/*
BackdoorInput carries retained causal rows.
*/
type BackdoorInput struct {
	Rows [][]float64
}

/*
BackdoorOutput reports association, adjusted effect, and conditioning.
*/
type BackdoorOutput struct {
	Value       float64
	Association float64
	Effect      float64
	Condition   float64
}

/*
Backdoor estimates a linear backdoor-adjusted treatment effect from rows.
*/
type Backdoor struct {
	config BackdoorConfig
}

/*
NewBackdoor returns a typed backdoor estimator.
*/
func NewBackdoor(config BackdoorConfig) *Backdoor {
	return &Backdoor{
		config: config,
	}
}

/*
Measure computes the adjusted treatment effect from retained rows.
*/
func (backdoor *Backdoor) Measure(input BackdoorInput) (BackdoorOutput, error) {
	table, err := newNodeTable(input.Rows, backdoor.config.Target)
	if err != nil {
		return BackdoorOutput{}, errnie.Error(errnie.Err(
			errnie.Validation,
			"causal backdoor: table construction failed",
			err,
		))
	}

	association, err := table.association(backdoor.config.Treatment)
	if err != nil {
		return BackdoorOutput{}, errnie.Error(errnie.Err(
			errnie.Validation,
			"causal backdoor: association failed",
			err,
		))
	}

	effect, err := table.backdoorEffect(
		backdoor.config.Treatment,
		backdoor.config.Controls...,
	)
	if err != nil {
		return BackdoorOutput{}, errnie.Error(errnie.Err(
			errnie.Validation,
			"causal backdoor: effect estimation failed",
			err,
		))
	}

	condition, err := table.pairConditionNumber(
		backdoor.config.Treatment,
		backdoor.config.Target,
	)
	if err != nil {
		return BackdoorOutput{}, errnie.Error(errnie.Err(
			errnie.Validation,
			"causal backdoor: pair condition failed",
			err,
		))
	}

	return BackdoorOutput{
		Value:       effect,
		Association: association,
		Effect:      effect,
		Condition:   condition,
	}, nil
}
