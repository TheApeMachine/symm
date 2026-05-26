package engine

const defaultSymbolConfidenceCap = 64

/*
SymbolConfidence maps raw signal strength into [0, 1] against symbol-local history.
Raw scores are recorded for fence calibration; normalized values are emitted downstream.
*/
type SymbolConfidence struct {
	history    []float64
	cap        int
	calibrator PredictionCalibrator
}

/*
NewSymbolConfidence returns a per-symbol confidence tracker.
*/
func NewSymbolConfidence(params CalibrationParams) *SymbolConfidence {
	return &SymbolConfidence{
		history:    make([]float64, 0, defaultSymbolConfidenceCap),
		cap:        defaultSymbolConfidenceCap,
		calibrator: NewPredictionCalibrator(params),
	}
}

/*
ApplyFeedback records settled forecast error into the parameter calibrator.
*/
func (tracker *SymbolConfidence) ApplyFeedback(feedback PredictionFeedback) {
	tracker.calibrator.Apply(feedback)
}

/*
Measure normalizes rawScore against prior raw history and records this observation.
Returns false until enough history exists or rawScore is non-positive.
*/
func (tracker *SymbolConfidence) Measure(rawScore float64) (float64, bool) {
	if rawScore <= 0 {
		return 0, false
	}

	confidence := tracker.calibrator.NormalizeConfidence(rawScore, tracker.history)
	tracker.record(rawScore)

	if confidence <= 0 {
		return 0, false
	}

	return confidence, true
}

func (tracker *SymbolConfidence) record(rawScore float64) {
	tracker.history = append(tracker.history, rawScore)

	capacity := tracker.cap

	if capacity <= 0 {
		capacity = defaultSymbolConfidenceCap
	}

	if len(tracker.history) > capacity {
		tracker.history = tracker.history[len(tracker.history)-capacity:]
	}
}

/*
WarmSymbolConfidence primes raw history so the next Measure can normalize.
*/
func WarmSymbolConfidence(tracker *SymbolConfidence, samples ...float64) {
	if tracker == nil {
		return
	}

	for _, sample := range samples {
		_, _ = tracker.Measure(sample)
	}
}
