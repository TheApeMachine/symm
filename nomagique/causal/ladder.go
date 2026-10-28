package causal

import (
	"errors"
	"io"

	"github.com/theapemachine/errnie"
)

/*
LadderConfig describes the causal role mapping and intervention settings.
*/
type LadderConfig struct {
	Target                 int
	TreatmentNormal        int
	ControlsNormal         []int
	TreatmentInverted      int
	ControlsInverted       []int
	InterventionLevel      float64
	InterventionPercentile float64
}

/*
LadderInput carries rows and the selected regime context.
*/
type LadderInput struct {
	Rows      [][]float64
	Inverted  bool
	Contagion float64
	Condition float64
}

/*
LadderOutput reports all Pearl-ladder evidence channels.
*/
type LadderOutput struct {
	Value             float64
	Association       float64
	Intervention      float64
	InterventionScore float64
	TreatmentScale    float64
	TargetScale       float64
	Uplift            float64
	UpliftScore       float64
	Contagion         float64
	Condition         float64
	Inverted          float64
}

/*
Ladder evaluates Pearl's ladder of causation over typed tabular rows.
*/
type Ladder struct {
	config LadderConfig
}

/*
NewLadder returns a typed ladder evaluator.
*/
func NewLadder(config LadderConfig) *Ladder {
	return &Ladder{
		config: config,
	}
}

/*
Measure evaluates association, intervention, and counterfactual uplift.
*/
func (ladder *Ladder) Measure(input LadderInput) (LadderOutput, error) {
	table, err := newNodeTable(input.Rows, ladder.config.Target)
	if err != nil {
		return LadderOutput{}, errnie.Error(errnie.Err(
			errnie.Validation,
			"causal ladder: table construction failed",
			err,
		))
	}

	treatment := ladder.config.TreatmentNormal
	controls := append([]int(nil), ladder.config.ControlsNormal...)

	if input.Inverted {
		treatment = ladder.config.TreatmentInverted
		controls = append([]int(nil), ladder.config.ControlsInverted...)
	}

	bandwidth, err := deriveBandwidth(input.Rows, controls...)
	if err != nil {
		return LadderOutput{}, err
	}

	if err := table.treatmentIdentifiable(treatment, controls...); err != nil {
		return LadderOutput{}, err
	}

	association, err := table.association(treatment)
	if err != nil {
		if errors.Is(err, io.EOF) {
			return LadderOutput{}, err
		}

		return LadderOutput{}, errnie.Error(errnie.Err(
			errnie.Validation,
			"causal ladder: association failed",
			err,
		))
	}

	intervention, err := table.kernelBackdoorEffect(
		treatment,
		bandwidth,
		controls...,
	)
	if err != nil {
		if errors.Is(err, io.EOF) {
			return LadderOutput{}, err
		}

		return LadderOutput{}, errnie.Error(errnie.Err(
			errnie.Validation,
			"causal ladder: kernel backdoor failed",
			err,
		))
	}

	uplift, err := ladder.uplift(table, input.Rows, treatment, controls)
	if err != nil {
		return LadderOutput{}, err
	}

	treatmentScale, targetScale, err := table.effectScales(treatment)
	if err != nil {
		return LadderOutput{}, err
	}

	interventionScore := intervention * treatmentScale / targetScale
	upliftScore := uplift / targetScale

	invertedValue := 0.0

	if input.Inverted {
		invertedValue = 1
	}

	return LadderOutput{
		Value:             intervention,
		Association:       association,
		Intervention:      intervention,
		InterventionScore: interventionScore,
		TreatmentScale:    treatmentScale,
		TargetScale:       targetScale,
		Uplift:            uplift,
		UpliftScore:       upliftScore,
		Contagion:         input.Contagion,
		Condition:         input.Condition,
		Inverted:          invertedValue,
	}, nil
}

func (ladder *Ladder) uplift(
	table nodeTable,
	rows [][]float64,
	treatment int,
	controls []int,
) (float64, error) {
	predictors := append(append([]int(nil), controls...), treatment)

	if len(rows) <= len(predictors) {
		return 0, io.EOF
	}

	currentRow := rows[len(rows)-1]
	interventionLevel, err := ladder.interventionLevel(table, treatment, rows)
	if err != nil {
		return 0, err
	}

	nonLinear, fitOK := fitNonLinearTable(table, predictors)
	if fitOK {
		return nonLinear.counterfactualUplift(
			currentRow,
			treatment,
			interventionLevel,
		)
	}

	linear, err := table.fitLinearModel(predictors...)
	if err != nil {
		if errors.Is(err, io.EOF) {
			return 0, io.EOF
		}

		return 0, errnie.Error(errnie.Err(
			errnie.Validation,
			"causal ladder: linear uplift fit failed",
			err,
		))
	}

	uplift, err := linear.counterfactualUplift(
		currentRow,
		treatment,
		interventionLevel,
	)
	if err != nil {
		return 0, errnie.Error(errnie.Err(
			errnie.Validation,
			"causal ladder: linear uplift failed",
			err,
		))
	}

	return uplift, nil
}

func (ladder *Ladder) interventionLevel(
	table nodeTable,
	treatment int,
	rows [][]float64,
) (float64, error) {
	if ladder.config.InterventionLevel > 0 {
		return ladder.config.InterventionLevel, nil
	}

	percentile := ladder.config.InterventionPercentile

	if percentile <= 0 {
		percentile = 1 - 1/float64(len(rows))
	}

	level, err := table.percentile(treatment, percentile)
	if err != nil {
		return 0, errnie.Error(errnie.Err(
			errnie.Validation,
			"causal ladder: intervention level failed",
			err,
		))
	}

	return level, nil
}
