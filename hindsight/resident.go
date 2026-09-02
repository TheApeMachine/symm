package hindsight

import (
	"sort"
	"strconv"
	"time"

	wire "github.com/theapemachine/symm/telemetry/generated/telemetry"
)

/*
Resident state: what the running system actually held at one capture
coordinate, as opposed to what one envelope happened to carry.

A stateful processor consumes more than the current Envelope (§18). A Flow
Perspective computed on a Level3 event reads a CVD last updated by an earlier
Trade, so the CVD it used has a different origin than the envelope that
triggered it. An inspection view that only shows the triggering envelope's own
artifacts therefore reports a signal as absent whenever this particular frame
did not recompute it — which reads as "SYMM did not know this", when the truth
is "SYMM knew it, from an earlier frame".

Resolution here is a causal as-of join, never a timestamp neighbourhood (§9):
for each signal family, the latest artifact whose origin CaptureSequence is at
or before the inspected coordinate. Every resolved fact reports the exact
capture it came from and how old it was, so a reader can always see that a
value is carried rather than fresh, and how far it was carried.

The future cannot contribute: an artifact produced after the coordinate is not
causally available there, and is never consulted (§31).
*/

/*
ResidentMetric is one metric of a resident measurement, with presence carried
explicitly exactly as the live artifact carried it.
*/
type ResidentMetric struct {
	Key             string  `json:"key"`
	Label           string  `json:"label,omitempty"`
	Raw             float64 `json:"raw"`
	Normalized      float64 `json:"normalized"`
	HasNormalized   bool    `json:"hasNormalized"`
	Standardized    float64 `json:"standardized"`
	HasStandardized bool    `json:"hasStandardized"`
	Unit            string  `json:"unit,omitempty"`
	Timescale       string  `json:"timescale,omitempty"`
}

/*
ResidentMeasurement is one signal family's resident value at the inspected
coordinate, together with the exact origin that produced it and its age there.

Age is the distance between the inspected coordinate's instant and the instant
the measurement itself declared. It is evidence about staleness, and it is the
reason a resident value can never be mistaken for a fresh one.
*/
type ResidentMeasurement struct {
	Source     string           `json:"source"`
	Identity   string           `json:"identity,omitempty"`
	Origin     EnvelopeRef      `json:"origin"`
	AtNs       int64            `json:"atNs"`
	AgeNs      int64            `json:"ageNs"`
	HasAge     bool             `json:"hasAge"`
	Carried    bool             `json:"carried"`
	Maturity   float64          `json:"maturity"`
	SNR        float64          `json:"snr"`
	SNRDefined bool             `json:"snrDefined"`
	Metrics    []ResidentMetric `json:"metrics"`
}

/*
ResidentCategory is one category hypothesis resident at the coordinate, with
the same origin and age accounting.
*/
type ResidentCategory struct {
	Type        string      `json:"type"`
	Origin      EnvelopeRef `json:"origin"`
	AgeNs       int64       `json:"ageNs"`
	HasAge      bool        `json:"hasAge"`
	Carried     bool        `json:"carried"`
	Confidence  float64     `json:"confidence"`
	Strength    float64     `json:"strength"`
	Maturity    float64     `json:"maturity"`
	Uncertainty float64     `json:"uncertainty"`
	Supporting  []string    `json:"supporting,omitempty"`
	Opposing    []string    `json:"opposing,omitempty"`
}

/*
ResidentPerspective preserves one retired metric-bucket Perspective from a
historical capture. It is decode-only compatibility evidence and is not the
current falsifiable Perspective contract. Kind is the legacy advisor-family
byte carried on that wire.
*/
type ResidentPerspective struct {
	Symbol   string            `json:"symbol"`
	Peer     string            `json:"peer,omitempty"`
	Kind     uint8             `json:"kind"`
	Origin   EnvelopeRef       `json:"origin"`
	AgeNs    int64             `json:"ageNs"`
	HasAge   bool              `json:"hasAge"`
	Carried  bool              `json:"carried"`
	Readings []ResidentReading `json:"readings"`
}

/*
ResidentReading is one constituent reading of a retired wire Perspective, with
its historical temporal/evidence attributes preserved verbatim. ObservedAt/From
are declared instants in nanoseconds; presence is not synthesised when the
recorded artifact left them undefined.
*/
type ResidentReading struct {
	Metric     string  `json:"metric"`
	Value      float64 `json:"value"`
	Defined    bool    `json:"defined"`
	ObservedAt int64   `json:"observedAt,omitempty"`
	HasAt      bool    `json:"hasAt"`
	From       int64   `json:"from,omitempty"`
	HasFrom    bool    `json:"hasFrom"`
	Maturity   float64 `json:"maturity"`
	SNR        float64 `json:"snr"`
	SNRDefined bool    `json:"snrDefined"`
}

/*
ResidentState is the assembled as-of view for one coordinate.

Unresolved names the signal families that were never found within the search
budget. That is deliberately not the same statement as "the system held
nothing": it says the search did not reach back far enough, and the caller is
told how far it looked so the distinction stays visible (§43, §48).
*/
type ResidentState struct {
	Run        RunID                 `json:"run"`
	Symbol     string                `json:"symbol"`
	Sequence   CaptureSequence       `json:"sequence"`
	Ordinal    uint64                `json:"ordinal"`
	At         time.Time             `json:"at"`
	Signals    []ResidentMeasurement `json:"signals"`
	Categories []ResidentCategory    `json:"categories"`
	Views      []ResidentPerspective `json:"perspectives"`

	Examined   int      `json:"examined"`
	Reached    uint64   `json:"reachedBack"`
	Exhausted  bool     `json:"exhausted"`
	Unresolved []string `json:"unresolved,omitempty"`
}

/*
StateReader reads the historical EnvelopeState persisted for one exact
EnvelopeRef. It is the narrow slice of the store the as-of resolution needs,
kept as an interface so this package never depends on a storage engine.
*/
type StateReader interface {
	ReadStatePayload(run string, sequence uint64, ordinal uint64) ([]byte, bool, error)
}

/*
residentSources is the set of signal families an as-of view resolves. It
mirrors the EnvelopeState schema rather than a hand-kept list of what happens
to be wired today, so a family that stops being produced shows as unresolved
instead of silently vanishing from the comparison.
*/
var residentSources = []string{
	"cvd",
	"hawkes",
	"depthFlow",
	"morphology",
	"liquidity",
	"correlation",
	"leadLag",
	"sentiment",
	"pumpDump",
	"toxicity",
	"derivatives",
}

/*
ResolveResident walks the instrument's own captures backwards from the
inspected coordinate and takes, for each signal family, the first value it
finds — which is by construction the latest one causally available there.

target is the complete EnvelopeRef being inspected: causal availability is
lexicographic over (CaptureSequence, Ordinal). An envelope with the same
CaptureSequence but a higher ordinal was produced by the same raw capture and
is future with respect to target — never consulted (§12, §31).

candidates must be the instrument's envelopes at or before the coordinate, in
descending causal order (CaptureSequence and then Ordinal descending). budget
bounds how far back the walk may reach; a walk that ends without resolving
every family reports what is missing rather than presenting a partial view as
complete.
*/
func ResolveResident(
	run RunID,
	symbol string,
	target EnvelopeRef,
	at time.Time,
	candidates []EnvelopeRef,
	reader StateReader,
	budget int,
) (ResidentState, error) {
	sequence := target.Origin.Sequence
	ordinal := target.Ordinal

	resident := ResidentState{
		Run:        run,
		Symbol:     symbol,
		Sequence:   sequence,
		Ordinal:    ordinal,
		At:         at,
		Signals:    make([]ResidentMeasurement, 0, len(residentSources)),
		Categories: make([]ResidentCategory, 0),
		Views:      make([]ResidentPerspective, 0),
	}

	if reader == nil || len(candidates) == 0 {
		resident.Unresolved = append([]string{}, residentSources...)
		return resident, nil
	}

	if budget <= 0 {
		budget = 64
	}

	found := make(map[string]bool, len(residentSources))
	seenCategory := make(map[string]bool)
	seenView := make(map[string]bool)
	markNs := at.UnixNano()

	for _, candidate := range candidates {
		if resident.Examined >= budget {
			resident.Exhausted = true
			break
		}

		if causalAfter(candidate, target) {
			// The future may select a coordinate but may never contribute state
			// to it. A candidate after the mark is skipped outright.
			continue
		}

		payload, present, err := reader.ReadStatePayload(
			string(run),
			uint64(candidate.Origin.Sequence),
			candidate.Ordinal,
		)

		if err != nil {
			return resident, err
		}

		resident.Examined++
		resident.Reached = uint64(sequence - candidate.Origin.Sequence)

		if !present || len(payload) == 0 {
			continue
		}

		state := wire.GetRootAsEnvelopeState(payload, 0)

		if state == nil {
			continue
		}

		carried := candidate.Origin.Sequence != sequence || candidate.Ordinal != ordinal

		for _, source := range residentSources {
			if found[source] {
				continue
			}

			measurement := measurementOf(state, source)

			if measurement == nil {
				continue
			}

			found[source] = true
			resident.Signals = append(
				resident.Signals,
				readResidentMeasurement(source, measurement, candidate, markNs, carried),
			)
		}

		for index := 0; index < state.CategoriesLength(); index++ {
			var category wire.EnvelopeCategory

			if !state.Categories(&category, index) {
				continue
			}

			name := string(category.Type())

			if name == "" || seenCategory[name] {
				continue
			}

			seenCategory[name] = true
			age := markNs - category.AtNs()

			resident.Categories = append(resident.Categories, ResidentCategory{
				Type:        name,
				Origin:      candidate,
				AgeNs:       age,
				HasAge:      category.AtNs() > 0,
				Carried:     carried,
				Confidence:  category.Confidence(),
				Strength:    category.Strength(),
				Maturity:    category.Maturity(),
				Uncertainty: category.Uncertainty(),
				Supporting:  stringList(category.SupportingLength(), category.Supporting),
				Opposing:    stringList(category.OpposingLength(), category.Opposing),
			})
		}

		for index := 0; index < state.PerspectivesLength(); index++ {
			var frame wire.PerspectiveFrame

			if !state.Perspectives(&frame, index) {
				continue
			}

			key := string(frame.Symbol()) + "|" + string(frame.Peer()) +
				"|" + strconv.Itoa(int(frame.Kind()))

			if seenView[key] {
				continue
			}

			seenView[key] = true
			readings := make([]ResidentReading, 0, frame.ReadingsLength())

			for slot := 0; slot < frame.ReadingsLength(); slot++ {
				var reading wire.PerspectiveReading

				if !frame.Readings(&reading, slot) {
					continue
				}

				readings = append(readings, ResidentReading{
					Metric:     string(reading.Metric()),
					Value:      reading.Value(),
					Defined:    reading.Defined(),
					ObservedAt: reading.ObservedAt(),
					HasAt:      reading.ObservedAt() > 0,
					From:       reading.From(),
					HasFrom:    reading.From() > 0,
					Maturity:   reading.Maturity(),
					SNR:        reading.Snr(),
					SNRDefined: reading.SnrDefined(),
				})
			}

			resident.Views = append(resident.Views, ResidentPerspective{
				Symbol:   string(frame.Symbol()),
				Peer:     string(frame.Peer()),
				Kind:     frame.Kind(),
				Origin:   candidate,
				AgeNs:    markNs - frame.At(),
				HasAge:   frame.At() > 0,
				Carried:  carried,
				Readings: readings,
			})
		}

		if len(found) == len(residentSources) {
			break
		}
	}

	for _, source := range residentSources {
		if !found[source] {
			resident.Unresolved = append(resident.Unresolved, source)
		}
	}

	sort.SliceStable(resident.Signals, func(left, right int) bool {
		return resident.Signals[left].Source < resident.Signals[right].Source
	})

	sort.SliceStable(resident.Categories, func(left, right int) bool {
		return resident.Categories[left].Type < resident.Categories[right].Type
	})

	return resident, nil
}

func readResidentMeasurement(
	source string,
	measurement *wire.EnvelopeMeasurement,
	origin EnvelopeRef,
	markNs int64,
	carried bool,
) ResidentMeasurement {
	metrics := make([]ResidentMetric, 0, measurement.MetricsLength())

	for index := 0; index < measurement.MetricsLength(); index++ {
		var entry wire.EnvelopeMeasurementMetric

		if !measurement.Metrics(&entry, index) {
			continue
		}

		metric := entry.Value(nil)

		if metric == nil {
			continue
		}

		metrics = append(metrics, ResidentMetric{
			Key:             string(entry.Key()),
			Label:           string(metric.Label()),
			Raw:             metric.Raw(),
			Normalized:      metric.Normalized(),
			HasNormalized:   metric.HasNormalized(),
			Standardized:    metric.Standardized(),
			HasStandardized: metric.HasStandardized(),
			Unit:            string(metric.Unit()),
			Timescale:       string(metric.Timescale()),
		})
	}

	return ResidentMeasurement{
		Source:     source,
		Identity:   string(measurement.Id()),
		Origin:     origin,
		AtNs:       measurement.AtNs(),
		AgeNs:      markNs - measurement.AtNs(),
		HasAge:     measurement.AtNs() > 0,
		Carried:    carried,
		Maturity:   measurement.Maturity(),
		SNR:        measurement.Snr(),
		SNRDefined: measurement.SnrDefined(),
		Metrics:    metrics,
	}
}

/*
measurementOf returns one signal family's measurement from a decoded state, or
nil when this envelope carried none.
*/
func measurementOf(state *wire.EnvelopeState, source string) *wire.EnvelopeMeasurement {
	measurement := new(wire.EnvelopeMeasurement)

	var present *wire.EnvelopeMeasurement

	switch source {
	case "cvd":
		present = state.Cvd(measurement)
	case "hawkes":
		present = state.Hawkes(measurement)
	case "depthFlow":
		present = state.DepthFlow(measurement)
	case "morphology":
		present = state.Morphology(measurement)
	case "liquidity":
		present = state.Liquidity(measurement)
	case "correlation":
		present = state.Correlation(measurement)
	case "leadLag":
		present = state.LeadLag(measurement)
	case "sentiment":
		present = state.Sentiment(measurement)
	case "pumpDump":
		present = state.PumpDump(measurement)
	case "toxicity":
		present = state.Toxicity(measurement)
	case "derivatives":
		present = state.Derivatives(measurement)
	}

	return present
}

func stringList(length int, at func(int) []byte) []string {
	if length == 0 {
		return nil
	}

	values := make([]string, 0, length)

	for index := 0; index < length; index++ {
		values = append(values, string(at(index)))
	}

	return values
}

/*
causalAfter reports whether candidate is strictly future with respect to target
under the complete causal ordering: (CaptureSequence, Ordinal) lexicographic.
A candidate with the same sequence but a greater ordinal is future, and a
candidate with a greater sequence is future regardless of ordinal.
*/
func causalAfter(candidate, target EnvelopeRef) bool {
	if candidate.Origin.Sequence != target.Origin.Sequence {
		return candidate.Origin.Sequence > target.Origin.Sequence
	}

	return candidate.Ordinal > target.Ordinal
}
