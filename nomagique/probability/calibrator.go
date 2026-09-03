package probability

/*
CalibratorConfig configures an empirical error calibrator.
*/
type CalibratorConfig struct {
	Window int
}

/*
CalibratorOutput reports the empirical percentile rank against retained history.
*/
type CalibratorOutput struct {
	Value float64
	Ready bool
	Count int
}

/*
Calibrator reports where an incoming error reading falls within a rolling empirical
distribution of recent observations, expressed as the fraction of retained history
the reading beats (where smaller error is better). Scoring against the prior window
ensures an observation cannot inflate its own rank.
*/
type Calibrator struct {
	config  CalibratorConfig
	samples []float64
	next    int
	filled  bool
}

/*
NewCalibrator returns an empirical error calibrator.
If no explicit window is configured, it grows dynamically with observations
without arbitrary static capacity clamps.
*/
func NewCalibrator(configs ...CalibratorConfig) *Calibrator {
	config := CalibratorConfig{}

	if len(configs) > 0 && configs[0].Window > 0 {
		config.Window = configs[0].Window
	}

	var initialCapacity int

	if config.Window > 0 {
		initialCapacity = config.Window
	}

	return &Calibrator{
		config:  config,
		samples: make([]float64, 0, initialCapacity),
	}
}

/*
Measure scores one reading against the prior window and folds it into the empirical distribution.
*/
func (calibrator *Calibrator) Measure(sample float64) (CalibratorOutput, error) {
	if err := finiteProbability("calibrator", sample); err != nil {
		return CalibratorOutput{}, err
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
		return CalibratorOutput{
			Value: 0,
			Ready: false,
			Count: 1,
		}, nil
	}

	return CalibratorOutput{
		Value: float64(beaten) / float64(count),
		Ready: true,
		Count: count + 1,
	}, nil
}

/*
Quantile scores one reading against the prior window and returns its empirical percentile.
*/
func (calibrator *Calibrator) Quantile(sample float64) float64 {
	out, err := calibrator.Measure(sample)

	if err != nil {
		return 0
	}

	return out.Value
}

/*
Reset clears all retained samples.
*/
func (calibrator *Calibrator) Reset() {
	calibrator.samples = calibrator.samples[:0]
	calibrator.next = 0
	calibrator.filled = false
}

/*
Count reports the number of committed samples currently retained.
*/
func (calibrator *Calibrator) Count() int {
	if calibrator.config.Window > 0 && calibrator.filled {
		return calibrator.config.Window
	}

	return len(calibrator.samples)
}
