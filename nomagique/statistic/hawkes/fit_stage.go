package hawkes

import "errors"

/*
fitStage estimates bivariate Hawkes parameters from timestamp arrival
streams.
*/
type fitStage struct {
	config fitStageConfig
}

/*
fitStageConfig carries the observation horizon and optional prior, in
seconds since an arbitrary but consistent epoch.
*/
type fitStageConfig struct {
	observedFromSec float64
	horizonSec      float64
	prior           bivariateFit
}

/*
fitStageInput carries timestamp arrival streams, in seconds since the same
epoch as fitStageConfig.
*/
type fitStageInput struct {
	xTimesSec []float64
	yTimesSec []float64
}

/*
fitStageOutput carries the fitted process and excitation evidence.
*/
type fitStageOutput struct {
	value           float64
	excitationRatio float64
	spectralRadius  float64
	asymmetry       float64
	fit             bivariateFit
}

/*
newFitStage creates a typed timestamp-stream Hawkes fit stage.
*/
func newFitStage(config fitStageConfig) (*fitStage, error) {
	if config.horizonSec == 0 {
		return nil, errors.New("hawkes-fit: horizon required")
	}

	return &fitStage{config: config}, nil
}

/*
measure estimates bivariate Hawkes parameters from arrival streams.
*/
func (stage *fitStage) measure(input fitStageInput) (fitStageOutput, error) {
	if len(input.xTimesSec)+len(input.yTimesSec) < 2 {
		return fitStageOutput{}, errors.New(
			"hawkes-fit: require aligned arrival timestamp streams",
		)
	}

	stream := newArrivalStreamFrom(stage.config.observedFromSec, input.xTimesSec, input.yTimesSec)

	if stream.observationOrigin() == 0 {
		stream = newArrivalStream(input.xTimesSec, input.yTimesSec)
	}

	fitted := newBivariateEstimator(stage.config.prior).fit(stream, stage.config.horizonSec)

	if !fitted.valid() {
		return fitStageOutput{}, errors.New(
			"hawkes-fit: fit did not converge to valid parameters",
		)
	}

	asymmetryValue := fitted.asymmetry(false)
	ratio := 0.0

	if asymmetryValue > 0 && fitted.muX > 0 {
		ratio = fitted.intensityX / fitted.muX
	}

	if asymmetryValue <= 0 && fitted.muY > 0 {
		ratio = fitted.intensityY / fitted.muY
	}

	return fitStageOutput{
		value:           ratio,
		excitationRatio: ratio,
		spectralRadius:  fitted.spectralRadius,
		asymmetry:       asymmetryValue,
		fit:             fitted,
	}, nil
}
