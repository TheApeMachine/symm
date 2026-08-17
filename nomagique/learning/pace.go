package learning

import (
	"math"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/probability"
)

const (
	defaultRestAlpha  = 0.03
	defaultMinAlpha   = 0.005
	defaultMaxAlpha   = 0.150
	defaultPaceGain   = 0.1
	defaultPaceBand   = 0.2
	defaultPaceWindow = 256
)

/*
PaceConfig configures an adaptive learning pace controller.
*/
type PaceConfig struct {
	InitialAlpha float64
	MinAlpha     float64
	MaxAlpha     float64
	Gain         float64
	Band         float64
	Window       int
}

/*
PaceOutput reports the adapted learning pace and current error rank.
*/
type PaceOutput struct {
	Alpha float64
	Rank  float64
	Ready bool
	Count int
}

/*
PaceController sets the learning pace from how surprising the current reconstruction
error is relative to its own recent empirical history.

The control signal is the rank of the reconstruction error within its own recent
history, integrated in log space with an explicit restoring term toward restAlpha.
Surprise raises the pace to follow regime changes faster, while unusually small
errors lower it to prevent fitting noise.
*/
type PaceController struct {
	config       PaceConfig
	restAlpha    float64
	currentAlpha float64
	minAlpha     float64
	maxAlpha     float64
	logAlpha     float64
	logRest      float64
	logMin       float64
	logMax       float64
	surprise     *probability.Calibrator
}

/*
NewPaceController constructs a dynamic pace controller bounded within [minAlpha, maxAlpha]
and resting at initialAlpha.
*/
func NewPaceController(configs ...PaceConfig) *PaceController {
	config := PaceConfig{
		InitialAlpha: defaultRestAlpha,
		MinAlpha:     defaultMinAlpha,
		MaxAlpha:     defaultMaxAlpha,
		Gain:         defaultPaceGain,
		Band:         defaultPaceBand,
		Window:       defaultPaceWindow,
	}

	if len(configs) > 0 {
		provided := configs[0]

		if provided.InitialAlpha > 0 {
			config.InitialAlpha = provided.InitialAlpha
		}

		if provided.MinAlpha > 0 {
			config.MinAlpha = provided.MinAlpha
		}

		if provided.MaxAlpha > 0 {
			config.MaxAlpha = provided.MaxAlpha
		}

		if provided.Gain > 0 {
			config.Gain = provided.Gain
		}

		if provided.Band > 0 {
			config.Band = provided.Band
		}

		if provided.Window > 0 {
			config.Window = provided.Window
		}
	}

	return &PaceController{
		config:       config,
		restAlpha:    config.InitialAlpha,
		currentAlpha: config.InitialAlpha,
		minAlpha:     config.MinAlpha,
		maxAlpha:     config.MaxAlpha,
		logAlpha:     math.Log(config.InitialAlpha),
		logRest:      math.Log(config.InitialAlpha),
		logMin:       math.Log(config.MinAlpha),
		logMax:       math.Log(config.MaxAlpha),
		surprise: probability.NewCalibrator(probability.CalibratorConfig{
			Window: config.Window,
		}),
	}
}

/*
Measure folds one reconstruction error into the retained history and updates the learning pace.
*/
func (controller *PaceController) Measure(reconstructionError float64) (PaceOutput, error) {
	if !finite(reconstructionError) {
		return PaceOutput{}, errnie.Error(errnie.Err(
			errnie.Validation,
			"pace: reconstruction error must be finite",
			nil,
		))
	}

	if controller.surprise.Count() < controller.config.Window {
		controller.surprise.Quantile(reconstructionError)

		return PaceOutput{
			Alpha: controller.currentAlpha,
			Rank:  0,
			Ready: false,
			Count: controller.surprise.Count(),
		}, nil
	}

	rank := controller.surprise.Quantile(reconstructionError)
	target := controller.logRest

	if rank < controller.config.Band {
		target = controller.logMax
	}

	if rank > 1.0-controller.config.Band {
		target = controller.logMin
	}

	controller.logAlpha += controller.config.Gain * (target - controller.logAlpha)
	controller.logAlpha = min(controller.logMax, max(controller.logMin, controller.logAlpha))
	controller.currentAlpha = math.Exp(controller.logAlpha)
	controller.currentAlpha = min(controller.maxAlpha, max(controller.minAlpha, controller.currentAlpha))

	return PaceOutput{
		Alpha: controller.currentAlpha,
		Rank:  rank,
		Ready: true,
		Count: controller.surprise.Count(),
	}, nil
}

/*
Update folds one error reading into the calibrator and returns the updated learning pace.
*/
func (controller *PaceController) Update(reconstructionError float64, temporalError ...float64) float64 {
	output, err := controller.Measure(reconstructionError)

	if err != nil {
		return controller.currentAlpha
	}

	return output.Alpha
}

/*
Alpha returns the current learning pace.
*/
func (controller *PaceController) Alpha() float64 {
	return controller.currentAlpha
}

/*
Bounds returns the pace floor and ceiling the controller integrates between.

A pace sitting on either rail is a controller that has run out of room rather
than one that has settled, so a reader of Alpha cannot tell the two apart
without the interval it moves inside.
*/
func (controller *PaceController) Bounds() (float64, float64) {
	return controller.minAlpha, controller.maxAlpha
}

/*
Count returns the number of retained observations in the error calibrator.
*/
func (controller *PaceController) Count() int {
	return controller.surprise.Count()
}

/*
Reset clears the retained calibrator history and returns alpha to restAlpha.
*/
func (controller *PaceController) Reset() {
	controller.currentAlpha = controller.restAlpha
	controller.logAlpha = controller.logRest
	controller.surprise.Reset()
}
