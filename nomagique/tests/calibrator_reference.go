// Test-only source calibrator used by the independent pace oracle.
package tests

import "fmt"

/*
referencePaceCalibratorConfig configures an empirical error calibrator.
*/
type referencePaceCalibratorConfig struct {
	Window int
}

/*
referencePaceCalibratorOutput reports the empirical percentile rank against retained history.
*/
type referencePaceCalibratorOutput struct {
	Value float64
	Ready bool
	Count int
}

/*
referencePaceCalibrator reports where an incoming error reading falls within a rolling empirical
distribution of recent observations, expressed as the fraction of retained history
the reading beats (where smaller error is better). Scoring against the prior window
ensures an observation cannot inflate its own rank.
*/
type referencePaceCalibrator struct {
	config  referencePaceCalibratorConfig
	samples []float64
	next    int
	filled  bool
}

/*
referencePaceNewCalibrator returns an empirical error calibrator.
If no explicit window is configured, it grows dynamically with observations
without arbitrary static capacity clamps.
*/
func referencePaceNewCalibrator(configs ...referencePaceCalibratorConfig) *referencePaceCalibrator {
	config := referencePaceCalibratorConfig{}

	if len(configs) > 0 && configs[0].Window > 0 {
		config.Window = configs[0].Window
	}

	var initialCapacity int

	if config.Window > 0 {
		initialCapacity = config.Window
	}

	return &referencePaceCalibrator{
		config:  config,
		samples: make([]float64, 0, initialCapacity),
	}
}

/*
Measure scores one reading against the prior window and folds it into the empirical distribution.
*/
func (calibrator *referencePaceCalibrator) Measure(sample float64) (referencePaceCalibratorOutput, error) {
	if err := referencePaceFinite("calibrator", sample); err != nil {
		return referencePaceCalibratorOutput{}, err
	}

	count := calibrator.Count()
	beaten := 0

	for index := range count {
		if sample < calibrator.samples[index] {
			beaten++
		}
	}

	if calibrator.config.Window > 0 {
		if len(calibrator.samples) < calibrator.config.Window {
			calibrator.samples = append(calibrator.samples, sample)
		} else {
			calibrator.samples[calibrator.next] = sample
		}

		calibrator.next = (calibrator.next + 1) % calibrator.config.Window

		if calibrator.next == 0 {
			calibrator.filled = true
		}
	} else {
		calibrator.samples = append(calibrator.samples, sample)
	}

	if count == 0 {
		return referencePaceCalibratorOutput{
			Value: 0,
			Ready: false,
			Count: 1,
		}, nil
	}

	return referencePaceCalibratorOutput{
		Value: float64(beaten) / float64(count),
		Ready: true,
		Count: count + 1,
	}, nil
}

/*
Quantile scores one reading against the prior window and returns its empirical percentile.
*/
func (calibrator *referencePaceCalibrator) Quantile(sample float64) float64 {
	out, err := calibrator.Measure(sample)

	if err != nil {
		return 0
	}

	return out.Value
}

/*
Reset clears all retained samples.
*/
func (calibrator *referencePaceCalibrator) Reset() {
	calibrator.samples = calibrator.samples[:0]
	calibrator.next = 0
	calibrator.filled = false
}

/*
Count reports the number of committed samples currently retained.
*/
func (calibrator *referencePaceCalibrator) Count() int {
	if calibrator.config.Window > 0 && calibrator.filled {
		return calibrator.config.Window
	}

	return len(calibrator.samples)
}

func referencePaceFinite(name string, value float64) error {
	if !referenceRLSFinite(value) {
		return fmt.Errorf("%s: nonfinite", name)
	}
	return nil
}
