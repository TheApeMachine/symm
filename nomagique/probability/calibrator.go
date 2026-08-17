package probability

const (
	defaultCalibratorWindow = 256
)

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
NewCalibrator returns an empirical error calibrator over the configured ring window.
*/
func NewCalibrator(configs ...CalibratorConfig) *Calibrator {
	config := CalibratorConfig{
		Window: defaultCalibratorWindow,
	}

	if len(configs) > 0 && configs[0].Window > 0 {
		config.Window = configs[0].Window
	}

	return &Calibrator{
		config:  config,
		samples: make([]float64, config.Window),
	}
}

/*
Measure scores one reading against the prior window and folds it into the ring.
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

	calibrator.samples[calibrator.next] = sample
	calibrator.next = (calibrator.next + 1) % len(calibrator.samples)

	if calibrator.next == 0 {
		calibrator.filled = true
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
Quantile computes the empirical rank of a sample and updates the retained window.
*/
func (calibrator *Calibrator) Quantile(sample float64) float64 {
	output, err := calibrator.Measure(sample)

	if err != nil {
		return 0
	}

	return output.Value
}

/*
Count returns how many observations the retained window currently holds.
*/
func (calibrator *Calibrator) Count() int {
	if calibrator.filled {
		return len(calibrator.samples)
	}

	return calibrator.next
}

/*
Reset clears the retained observation history.
*/
func (calibrator *Calibrator) Reset() {
	calibrator.next = 0
	calibrator.filled = false

	for index := range calibrator.samples {
		calibrator.samples[index] = 0
	}
}
