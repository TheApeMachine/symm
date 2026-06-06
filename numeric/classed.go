package numeric

import (
	"github.com/theapemachine/symm/numeric/adaptive"
)

type Classed struct {
	derived    *Derived
	classifier *adaptive.Classifier
	classify   *Classify
}

func NewClassed(classifier *adaptive.Classifier, stages ...Dynamic) *Classed {
	classify := NewClassify(classifier)

	needsProject := true

	if len(stages) > 0 {
		if _, ok := stages[0].(*Project); ok {
			needsProject = false
		}
	}

	var dynamics []Dynamic

	if needsProject {
		passthrough := NewProjectScalar(func(_ float64, values []float64) float64 {
			if len(values) == 0 {
				return 0
			}

			return values[0]
		})
		dynamics = append([]Dynamic{passthrough}, stages...)
	} else {
		dynamics = append([]Dynamic(nil), stages...)
	}

	dynamics = append(dynamics, classify)

	return &Classed{
		derived:    NewDerived(WithDynamics(dynamics...)),
		classifier: classifier,
		classify:   classify,
	}
}

func (classed *Classed) Push(values ...float64) (float64, error) {
	return classed.derived.Push(values...)
}

func (classed *Classed) Label(code float64) string {
	return classed.classifier.Label(code)
}

/*
Confidence returns how clearly the last Push landed in its category — see
adaptive.Classifier.Confidence.
*/
func (classed *Classed) Confidence() float64 {
	if classed == nil || classed.classifier == nil {
		return 0
	}

	return classed.classifier.Confidence(classed.classify.lastObservation)
}

func (classed *Classed) Standout() float64 {
	if classed == nil || classed.classifier == nil {
		return 0
	}

	return classed.classifier.Standout(classed.classify.lastObservation)
}

/*
Observation returns the scalar the classifier banded on the last Push — the
post-projection, post-clamp value. It is what self-calibration fits the band
edges to, and what a raw dump records to make the signal observable.
*/
func (classed *Classed) Observation() float64 {
	if classed == nil || classed.classify == nil {
		return 0
	}

	return classed.classify.lastObservation
}

/*
Telemetry is a live snapshot of a self-calibrating classifier's state for the
dashboard: the current band edges, their labels, the recent category mix, the last
banded observation, and whether calibration has refit yet.
*/
type Telemetry struct {
	Edges        []float64 `json:"bands"`
	Labels       []string  `json:"labels"`
	Shares       []float64 `json:"shares"`
	Observation  float64   `json:"observation"`
	Calibrating  bool      `json:"calibrating"`
	Calibrated   bool      `json:"calibrated"`
	Samples      int       `json:"samples"`
	MinSamples   int       `json:"min_samples"`
	EntropyTrust float64   `json:"entropy_trust"`
}

func (classed *Classed) Reset() error {
	return classed.derived.Reset()
}

type Classify struct {
	classifier      *adaptive.Classifier
	lastObservation float64
}

func NewClassify(classifier *adaptive.Classifier) *Classify {
	return &Classify{classifier: classifier}
}

func (classify *Classify) Next(out float64, values ...float64) (float64, error) {
	_ = values
	classify.lastObservation = out

	return classify.classifier.Code(out)
}

func (classify *Classify) Reset() error {
	return nil
}
