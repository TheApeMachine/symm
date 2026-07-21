package manifold

import (
	"math"
	"time"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/nomagique/geometry"
	"github.com/theapemachine/symm/types"
)

/*
PhaseCorpus owns pending resident fingerprints and the outcome-labeled history
for each focused symbol. Its symbol boundary prevents categorical responses
from one market being projected onto another market's compass.
*/
type PhaseCorpus struct {
	capacity int
	spectra  map[string]*geometry.Corpus[PhaseOutcome]
	pending  map[string]phaseSnapshot
}

/*
phaseSnapshot holds one resident field until cognition labels the same symbol
epoch. Unclassified visualization frames never enter the searchable history.
*/
type phaseSnapshot struct {
	dial geometry.PhaseDial
	at   time.Time
}

/*
NewPhaseCorpus creates symbol-scoped histories at the configured market event
capacity so phase retention obeys the same explicit bound as source history.
*/
func NewPhaseCorpus(capacity int) (*PhaseCorpus, error) {
	if capacity <= 0 {
		return nil, errnie.Err(
			errnie.Validation,
			"manifold: invalid spectral history capacity",
			nil,
		)
	}

	return &PhaseCorpus{
		capacity: capacity,
		spectra:  make(map[string]*geometry.Corpus[PhaseOutcome]),
		pending:  make(map[string]phaseSnapshot),
	}, nil
}

/*
Stage holds the current focused fingerprint until the classifier supplies the
outcome for this exact symbol epoch.
*/
func (corpus *PhaseCorpus) Stage(
	symbol string,
	at time.Time,
	dial geometry.PhaseDial,
) {
	corpus.pending[symbol] = phaseSnapshot{dial: dial, at: at}
}

/*
CommitPhase retains a pending field only when DMT produced a ready outcome for
the exact focused symbol epoch.
*/
func (corpus *PhaseCorpus) CommitPhase(cognition types.Cognition) error {
	if corpus == nil || !cognition.Ready {
		return nil
	}

	pending, found := corpus.pending[cognition.Symbol]

	if !found || !pending.at.Equal(cognition.At) {
		return nil
	}

	spectrum := corpus.spectra[cognition.Symbol]

	if spectrum == nil {
		created, err := geometry.NewCorpus[PhaseOutcome](corpus.capacity)

		if err != nil {
			return errnie.Err(
				errnie.Validation,
				"manifold: failed to create symbol phase corpus",
				err,
			)
		}

		spectrum = created
		corpus.spectra[cognition.Symbol] = spectrum
	}

	err := spectrum.Insert(geometry.CorpusEntry[PhaseOutcome]{
		Dial: pending.dial,
		Outcome: PhaseOutcome{
			Symbol:     cognition.Symbol,
			Class:      cognition.Winner,
			Confidence: cognition.Confidence,
			Ambiguous:  cognition.Ambiguous,
			Cohort:     cognition.Cohort,
		},
		At: cognition.At,
	})

	if err != nil {
		return errnie.Err(
			errnie.UnprocessableContent,
			"manifold: failed to retain outcome-labeled phase dial",
			err,
		)
	}

	delete(corpus.pending, cognition.Symbol)
	return nil
}

/*
Responses scans a complete mode-derived turn and returns the strongest prior
response and its actual DMT outcome at every sampled angle.
*/
func (corpus *PhaseCorpus) Responses(
	symbol string,
	dial geometry.PhaseDial,
	at time.Time,
) ([]PhaseResponse, error) {
	spectrum := corpus.spectra[symbol]

	if spectrum == nil || spectrum.Size() == 0 {
		return nil, nil
	}

	angles := make([]float64, len(dial))

	for index := range angles {
		angles[index] = 2 * math.Pi * float64(index) / float64(len(dial))
	}

	responses, err := spectrum.ScanPhasesExcluding(dial, angles, 1, at)

	if err != nil {
		return nil, errnie.Err(
			errnie.UnprocessableContent,
			"manifold: failed to scan resident phase dial",
			err,
		)
	}

	phaseScan := make([]PhaseResponse, 0, len(responses))

	for index, matches := range responses {
		if len(matches) == 0 {
			continue
		}

		phaseScan = append(phaseScan, PhaseResponse{
			Angle:      angles[index],
			Similarity: matches[0].Similarity,
			ObservedAt: matches[0].At,
			Outcome:    matches[0].Outcome,
		})
	}

	return phaseScan, nil
}
