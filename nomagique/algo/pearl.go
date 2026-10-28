package algo

import (
	"errors"
	"io"
	"math"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/causal"
	"github.com/theapemachine/symm/nomagique/probability"
)

/*
Pearl evaluates Pearl's causal ladder over keyed numeric observations.
It delegates the causal math to the causal package and owns only row history
and evidence classification.
*/
type Pearl struct {
	config     PearlConfig
	sample     *PearlSample
	ladder     *causal.Ladder
	classifier *probability.ScoreClassifier
}

/*
PearlConfig configures numeric causal-ladder evaluation.
*/
type PearlConfig struct {
	Target                  int
	Treatment               int
	Controls                []int
	TreatmentInverted       int
	ControlsInverted        []int
	InterventionLevel       float64
	InterventionPercentile  float64
	NonlinearCounterfactual bool
}

/*
PearlOutput contains causal ladder, do, and counterfactual evidence.
*/
type PearlOutput struct {
	Value              float64            `json:"value"`
	Category           float64            `json:"category"`
	Confidence         float64            `json:"confidence"`
	ConfidenceBaseline float64            `json:"confidenceBaseline"`
	EntryBaseline      float64            `json:"entryBaseline"`
	ExitBaseline       float64            `json:"exitBaseline"`
	Strength           float64            `json:"strength"`
	Association        float64            `json:"association"`
	AssociationScore   float64            `json:"associationScore"`
	Intervention       float64            `json:"intervention"`
	InterventionScore  float64            `json:"interventionScore"`
	DoExpectation      float64            `json:"doExpectation"`
	Uplift             float64            `json:"uplift"`
	UpliftScore        float64            `json:"upliftScore"`
	Counterfactual     float64            `json:"counterfactual"`
	Noise              float64            `json:"noise"`
	NoiseScore         float64            `json:"noiseScore"`
	Contagion          float64            `json:"contagion"`
	Condition          float64            `json:"condition"`
	Inverted           bool               `json:"inverted"`
	Probabilities      []float64          `json:"probabilities"`
	Distribution       map[string]float64 `json:"distribution"`
}

/*
NewPearl returns a numeric Pearl-ladder calculator.
*/
func NewPearl(configs ...PearlConfig) *Pearl {
	config := PearlConfig{}

	if len(configs) > 0 {
		config = configs[0]
	}

	return &Pearl{
		config: config,
		sample: NewPearlSample(config),
		ladder: causal.NewLadder(causal.LadderConfig{
			Target:                 config.Target,
			TreatmentNormal:        config.Treatment,
			ControlsNormal:         config.Controls,
			TreatmentInverted:      config.TreatmentInverted,
			ControlsInverted:       config.ControlsInverted,
			InterventionLevel:      config.InterventionLevel,
			InterventionPercentile: config.InterventionPercentile,
		}),
		classifier: probability.NewScoreClassifier(
			[]string{"association", "intervention", "counterfactual", "residual"},
		),
	}
}

/*
Measure observes one numeric row and returns causal evidence when ready.
*/
func (pearl *Pearl) Measure(input PearlInput) (PearlOutput, bool, error) {
	sample, ready, err := pearl.sample.Measure(input)
	if err != nil || !ready {
		return PearlOutput{}, false, err
	}

	ladderOutput, err := pearl.ladder.Measure(causal.LadderInput{
		Rows:      sample.Rows,
		Inverted:  input.Inverted,
		Contagion: input.Contagion,
		Condition: input.Condition,
	})
	if err != nil {
		if errors.Is(err, io.EOF) {
			return PearlOutput{}, false, nil
		}

		return PearlOutput{}, false, err
	}

	table, err := causal.NewNodeTableWrapper(sample.Rows, pearl.config.Target)
	if err != nil {
		return PearlOutput{}, false, err
	}

	treatment := pearl.config.Treatment
	controls := append([]int(nil), pearl.config.Controls...)

	if input.Inverted {
		treatment = pearl.config.TreatmentInverted
		controls = append([]int(nil), pearl.config.ControlsInverted...)
	}

	if treatment < 0 || treatment >= len(sample.Row) {
		return PearlOutput{}, false, errnie.Error(errnie.Err(
			errnie.Validation,
			"pearl: treatment outside row width",
			nil,
		))
	}

	interventionLevel := input.Intervention

	if interventionLevel == 0 {
		interventionLevel = sample.Row[treatment]
	}

	doExpectation, err := table.DoExpectation(
		treatment,
		interventionLevel,
		controls...,
	)
	if err != nil {
		if errors.Is(err, io.EOF) {
			return PearlOutput{}, false, nil
		}

		return PearlOutput{}, false, err
	}

	features := append(append([]int(nil), controls...), treatment)
	uplift, counterfactual, noise, err := table.AbductiveCounterfactual(
		features,
		!pearl.config.NonlinearCounterfactual,
		sample.Row,
		pearl.config.Target,
		treatment,
		interventionLevel,
	)
	if err != nil {
		if errors.Is(err, io.EOF) {
			return PearlOutput{}, false, nil
		}

		return PearlOutput{}, false, err
	}

	output := PearlOutput{
		Association:       ladderOutput.Association,
		AssociationScore:  math.Abs(ladderOutput.Association),
		Intervention:      ladderOutput.Intervention,
		InterventionScore: math.Abs(ladderOutput.InterventionScore),
		DoExpectation:     doExpectation,
		Uplift:            uplift,
		UpliftScore:       math.Abs(uplift) / ladderOutput.TargetScale,
		Counterfactual:    counterfactual,
		Noise:             noise,
		NoiseScore:        math.Abs(noise) / ladderOutput.TargetScale,
		Contagion:         ladderOutput.Contagion,
		Condition:         ladderOutput.Condition,
		Inverted:          input.Inverted || ladderOutput.Inverted > 0,
	}

	result, err := pearl.classifier.Classify(map[string]float64{
		"association":    output.AssociationScore,
		"intervention":   output.InterventionScore,
		"counterfactual": output.UpliftScore,
		"residual":       output.Residual(),
		"strength":       output.evidenceStrength(),
	})
	if err != nil {
		return PearlOutput{}, false, errnie.Error(errnie.Err(
			errnie.Validation,
			"pearl: classification failed",
			err,
		))
	}

	output.Value = result.Value
	output.Category = result.Category
	output.Confidence = result.Confidence
	output.ConfidenceBaseline = result.ConfidenceBaseline
	output.EntryBaseline = result.EntryBaseline
	output.ExitBaseline = result.ExitBaseline
	output.Strength = result.Strength
	output.Probabilities = result.Probabilities
	output.Distribution = result.Distribution

	return output, true, nil
}

/*
Residual returns standardized unexplained target magnitude.
*/
func (output PearlOutput) Residual() float64 {
	return output.NoiseScore
}

/*
Outputs returns the map shape expected by signal measurement artifacts.
*/
func (output PearlOutput) Outputs() map[string]any {
	return map[string]any{
		"association":         output.Association,
		"associationScore":    output.AssociationScore,
		"intervention":        output.Intervention,
		"interventionScore":   output.InterventionScore,
		"doExpectation":       output.DoExpectation,
		"uplift":              output.Uplift,
		"upliftScore":         output.UpliftScore,
		"counterfactual":      output.Counterfactual,
		"noise":               output.Noise,
		"noiseScore":          output.NoiseScore,
		"residual":            output.Residual(),
		"contagion":           output.Contagion,
		"condition":           output.Condition,
		"inverted":            output.invertedValue(),
		"strength":            output.Strength,
		"value":               output.Value,
		"category":            output.Category,
		"confidence":          output.Confidence,
		"confidence_baseline": output.ConfidenceBaseline,
		"entry_baseline":      output.EntryBaseline,
		"exit_baseline":       output.ExitBaseline,
		"probabilities":       output.Probabilities,
		"distribution":        output.Distribution,
	}
}

func (output PearlOutput) evidenceStrength() float64 {
	return math.Max(
		output.AssociationScore,
		math.Max(output.InterventionScore, math.Max(output.UpliftScore, output.Residual())),
	)
}

func (output PearlOutput) invertedValue() float64 {
	if output.Inverted {
		return 1
	}

	return 0
}
