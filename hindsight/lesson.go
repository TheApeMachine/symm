package hindsight

import (
	"fmt"
	"time"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/data"
	wire "github.com/theapemachine/symm/telemetry/generated/telemetry"
)

/*
Lesson selects a symbol's complete captured session for rehearsal. Its marks
are retrospective evaluation coordinates, never decision inputs. Keeping the
whole session includes the lead-in, stagnation, reversal and quiet intervals;
a replay does not begin a fresh wallet precisely at a hindsight price minimum.
*/
type Lesson struct {
	Run     RunID           `json:"run"`
	Symbol  string          `json:"symbol"`
	From    CaptureSequence `json:"from"`
	Through CaptureSequence `json:"through"`
	Marks   []Episode       `json:"marks"`
	Horizon time.Duration   `json:"horizonNs"`
}

/*
Lessons selects confirmed spot price episodes using canonical Hindsight
geometry. Horizon is the mean completed leg duration, a training-data estimate
of the time scale to measure, not an entry or exit timestamp. The caller must
freeze it before evaluating later tape. Quiet intervals remain in every lesson.
*/
func (index *RunIndex) Lessons(policy DiscoveryPolicy) []Lesson {
	lessons := []Lesson{}
	for _, summary := range index.Summaries(policy) {
		lesson := Lesson{Run: index.run, Symbol: summary.Symbol, From: summary.FirstSeq, Through: summary.LastSeq}
		count := 0
		mean := 0.0
		for _, episode := range index.Discover(summary.Symbol, policy).Episodes {
			if !episode.Confirmed || (episode.Kind != EpisodeUpwardExcursion && episode.Kind != EpisodeDownwardExcursion) {
				continue
			}
			elapsed := episode.ToAt.Sub(episode.FromAt)
			if elapsed <= 0 {
				continue
			}
			count++
			mean += (float64(elapsed) - mean) / float64(count)
			lesson.Marks = append(lesson.Marks, episode)
		}
		if count == 0 {
			continue
		}
		lesson.Horizon = time.Duration(mean)
		lessons = append(lessons, lesson)
	}
	return lessons
}

/* RehearsalInput is an immutable numerical decision input, without hindsight labels or old actions. */
type RehearsalInput struct {
	Capture      CaptureIdentity              `json:"capture"`
	Symbol       string                       `json:"symbol"`
	At           time.Time                    `json:"at"`
	Measurements []*data.Measurement[float64] `json:"measurements"`
}

// RehearsalInput extracts only producer measurements from a captured witness.
// Old wallet state, actions, grades, and retrospective episode labels are never
// selection inputs. Source/metric identities and producer horizons are retained.
func (witness ArtifactWitness) RehearsalInput() (input RehearsalInput, err error) {
	defer func() {
		if invalid := recover(); invalid != nil {
			err = errnie.Error(errnie.Err(errnie.Validation,
				fmt.Sprintf("rehearsal: malformed captured precursor: %v", invalid), nil))
		}
	}()
	state := wire.GetRootAsEnvelopeState(witness.Payload, 0)

	if string(state.CaptureRun()) != string(witness.Envelope.Origin.Run) ||
		state.CaptureSeq() != uint64(witness.Envelope.Origin.Sequence) ||
		state.CaptureOrdinal() != witness.Envelope.Ordinal {
		return input, errnie.Error(errnie.Err(errnie.Validation, "rehearsal: precursor capture identity mismatch", nil))
	}
	if state.LearningObservationsLength() == 0 &&
		(state.CategoriesLength() > 0 || state.Cognition(nil) != nil || state.Resonance(nil) != nil || state.Manifold(nil) != nil) {
		return input, errnie.Error(errnie.Err(errnie.Validation,
			"rehearsal: legacy witness lacks captured numerical logic observations; recapture required", nil))
	}
	input.Capture, input.Symbol = witness.Envelope.Origin, string(state.Key())

	encodedMeasurements := make([]*wire.EnvelopeMeasurement, 0, len(residentSources)+state.LearningObservationsLength())
	if state.LearningObservationsLength() == 0 {
		for _, source := range residentSources {
			if encoded := measurementOf(state, source); encoded != nil {
				encodedMeasurements = append(encodedMeasurements, encoded)
			}
		}
	}
	for index := range state.LearningObservationsLength() {
		encoded := new(wire.EnvelopeMeasurement)
		if !state.LearningObservations(encoded, index) {
			return input, errnie.Error(errnie.Err(errnie.Validation, "rehearsal: absent numerical logic record", nil))
		}
		encodedMeasurements = append(encodedMeasurements, encoded)
	}
	for _, encoded := range encodedMeasurements {
		reading := encoded.UnPack()
		from := time.Time{}

		if reading.HasFrom {
			from = time.Unix(0, reading.FromNs)
		}
		measurement := data.NewMeasurement[float64](reading.Id, reading.Label, reading.Source,
			time.Unix(0, reading.AtNs), from)
		measurement.SeqIdx, measurement.Maturity = reading.SeqIdx, reading.Maturity
		measurement.SNR, measurement.SNRDefined = reading.Snr, reading.SnrDefined
		measurement.Metadata = make(map[string]float64, len(reading.Metadata))
		measurement.Provenance = make(map[string]string, len(reading.Provenance))

		for _, fact := range reading.Metadata {
			measurement.Metadata[fact.Name] = fact.Value
		}

		for _, fact := range reading.Provenance {
			measurement.Provenance[fact.Name] = fact.Value
		}

		for _, metric := range reading.Metrics {
			if metric == nil || metric.Value == nil {
				return input, errnie.Error(errnie.Err(errnie.Validation, "rehearsal: missing captured metric", nil))
			}
			value := metric.Value
			decoded := data.Metric[float64]{Label: value.Label, Raw: value.Raw,
				Unit: data.Unit(value.Unit), Timescale: data.Timescale(value.Timescale)}

			if value.HasNormalized {
				decoded.Normalized = &value.Normalized
			}

			if value.HasStandardized {
				decoded.Standardized = &value.Standardized
			}
			measurement.Metrics[metric.Key] = decoded
		}
		input.Measurements = append(input.Measurements, measurement)
	}
	return input, nil
}
