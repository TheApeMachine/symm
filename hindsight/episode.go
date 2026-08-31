package hindsight

import (
	"fmt"
	"math"
	"sort"
	"time"
)

/*
EpisodeKind names one objectively interesting region of historical market
behaviour (§26). Every kind here describes market or operational reality. None
of them describes what SYMM should have done, and none of them is selected by
consulting a SYMM trading output (§27).
*/
type EpisodeKind string

const (
	EpisodeUpwardExcursion       EpisodeKind = "upward_excursion"
	EpisodeDownwardExcursion     EpisodeKind = "downward_excursion"
	EpisodeReversal              EpisodeKind = "reversal"
	EpisodeVolatilityExpansion   EpisodeKind = "volatility_expansion"
	EpisodeVolatilityContraction EpisodeKind = "volatility_contraction"
	EpisodeSpreadExpansion       EpisodeKind = "spread_expansion"
	EpisodeLiquidityCollapse     EpisodeKind = "liquidity_collapse"
	EpisodeArrivalCluster        EpisodeKind = "arrival_cluster"
)

/*
ReferenceRole names one piece of retrospective geometry inside an Episode
(§28). A ReferencePoint is a coordinate on the historical record. It is NOT a
recommendation: an Anchor is the retrospective start of the selected excursion,
which never means SYMM should have bought there.
*/
type ReferenceRole string

const (
	ReferenceAnchor     ReferenceRole = "anchor"
	ReferencePeak       ReferenceRole = "peak"
	ReferenceTrough     ReferenceRole = "trough"
	ReferenceReversal   ReferenceRole = "reversal"
	ReferenceExitAnchor ReferenceRole = "exit_anchor"
	ReferenceShockOnset ReferenceRole = "shock_onset"
)

/*
ReferencePoint pins one retrospective coordinate to the exact capture identity
that carried it, so the inspection view assembled for it (§32) is addressed by
identity rather than by timestamp proximity (§9).
*/
type ReferencePoint struct {
	Role       ReferenceRole   `json:"role"`
	Capture    CaptureIdentity `json:"capture"`
	Ordinal    uint64          `json:"ordinal"`
	VenueAt    time.Time       `json:"venueAt"`
	ReceivedAt time.Time       `json:"receivedAt"`
	Value      float64         `json:"value"`
	HasValue   bool            `json:"hasValue"`
}

/*
Episode is one discovered interesting region of the captured market record.

ObservedExcursion is signed market geometry in the declared coordinate — the
fraction the coordinate moved between the episode's anchor and its extremum. It
is deliberately NOT called profit, capture, or missed anything (§30, §38): it
says the named coordinate moved, and nothing about executability.

Ratio is the regime magnitude for the regime kinds (realised-volatility ratio,
spread ratio, arrival-rate ratio, depth ratio) against the selector's declared
baseline. Only one of ObservedExcursion / Ratio is defined for a given kind,
and each carries its own presence flag rather than defaulting to zero (§43).
*/
type Episode struct {
	ID           string           `json:"id"`
	Symbol       string           `json:"symbol"`
	Kind         EpisodeKind      `json:"kind"`
	Coordinate   MarketCoordinate `json:"coordinate"`
	FromSequence CaptureSequence  `json:"fromSequence"`
	ToSequence   CaptureSequence  `json:"toSequence"`
	FromAt       time.Time        `json:"fromAt"`
	ToAt         time.Time        `json:"toAt"`
	Observations int              `json:"observations"`

	ObservedExcursion    float64 `json:"observedExcursion"`
	HasObservedExcursion bool    `json:"hasObservedExcursion"`

	// Confirmed reports whether the record itself closed this episode. A
	// price leg is confirmed by an observed retracement away from its
	// extremum; a regime span is confirmed by an observation that left the
	// regime. The last episode of a tape still being captured is
	// unconfirmed: the evidence for its end has not arrived yet (§33).
	Confirmed bool    `json:"confirmed"`
	Ratio     float64 `json:"ratio"`
	HasRatio  bool    `json:"hasRatio"`

	// Traversed is the total distance the declared coordinate actually
	// travelled across the episode's legs, as a fraction of the anchor. A
	// reversal returns near its anchor, so its net excursion is small while
	// the coordinate covered real ground; both are reported rather than
	// letting one stand in for the other.
	Traversed    float64 `json:"traversed"`
	HasTraversed bool    `json:"hasTraversed"`

	// Threshold is the selector value Ratio had to pass for this regime span
	// to qualify. It travels with the episode so a reader sees the ratio
	// against the bar it cleared, rather than a bare number whose selector
	// lives somewhere else (§29).
	Threshold    float64 `json:"threshold"`
	HasThreshold bool    `json:"hasThreshold"`

	References []ReferencePoint `json:"references"`
}

/*
Magnitude is the episode's size in its own terms: the distance the declared
coordinate travelled for price geometry, and the exceedance over its own
selector threshold for a regime span.

The two are deliberately NOT comparable, and nothing here pretends they are: a
58x arrival-rate burst is not "bigger" than a 20% excursion, it is a different
kind of fact. Callers ranking inspection targets rank within a kind (§30 —
language is part of the safety boundary; so is arithmetic).
*/
func (episode Episode) Magnitude() float64 {
	if episode.HasTraversed {
		return episode.Traversed
	}

	if episode.HasObservedExcursion {
		return math.Abs(episode.ObservedExcursion)
	}

	if !episode.HasRatio {
		return 0
	}

	if !episode.HasThreshold || episode.Threshold <= 0 {
		return 0
	}

	// An upper-bound selector qualifies above its threshold, a lower-bound
	// selector below it; exceedance is how far past the bar the span went, in
	// multiples of the bar itself.
	if episode.Threshold >= 1 {
		return episode.Ratio/episode.Threshold - 1
	}

	if episode.Ratio <= 0 {
		return 0
	}

	return episode.Threshold/episode.Ratio - 1
}

/*
IsPriceGeometry reports whether this episode describes movement of the declared
coordinate itself, as opposed to a regime of the surrounding microstructure.
*/
func (episode Episode) IsPriceGeometry() bool {
	switch episode.Kind {
	case EpisodeUpwardExcursion, EpisodeDownwardExcursion, EpisodeReversal:
		return true
	default:
		return false
	}
}

/*
Reference returns the episode's reference point for one role.
*/
func (episode Episode) Reference(role ReferenceRole) (ReferencePoint, bool) {
	for _, reference := range episode.References {
		if reference.Role == role {
			return reference, true
		}
	}

	return ReferencePoint{}, false
}

/*
DiscoveryPolicy declares the selector: which coordinate is measured and what
thresholds make a region qualify. Every threshold is part of the declared
selector and is reported back with the discovery result, so a reader always
knows exactly what "interesting" meant here — the selector is evidence, not a
hidden constant (§29, §33).

Excursion qualification is derived rather than fixed: the qualifying move is
ExcursionSigmas standard deviations of the symbol's own per-observation log
return, random-walk scaled over ExcursionHorizon observations, floored at
FloorExcursion so a numerically dead symbol cannot manufacture episodes out of
quantisation noise.
*/
type DiscoveryPolicy struct {
	Coordinate MarketCoordinate `json:"coordinate"`

	FloorExcursion    float64 `json:"floorExcursion"`
	ExcursionSigmas   float64 `json:"excursionSigmas"`
	ExcursionHorizon  int     `json:"excursionHorizon"`
	RetraceFraction   float64 `json:"retraceFraction"`
	RegimeWindow      int     `json:"regimeWindow"`
	RegimeBaseline    int     `json:"regimeBaseline"`
	VolatilityRatio   float64 `json:"volatilityRatio"`
	SpreadRatio       float64 `json:"spreadRatio"`
	DepthRatio        float64 `json:"depthRatio"`
	ArrivalRatio      float64 `json:"arrivalRatio"`
	MinRegimeSpan     int     `json:"minRegimeSpan"`
	MinObservations   int     `json:"minObservations"`
	MaxEpisodesPerSet int     `json:"maxEpisodesPerSet"`
}

/*
DefaultDiscoveryPolicy is the selector used when a caller declares none. It is
a declared default, reported verbatim alongside every result it produced.
*/
func DefaultDiscoveryPolicy() DiscoveryPolicy {
	return DiscoveryPolicy{
		Coordinate:        CoordinateMidpoint,
		FloorExcursion:    0.003,
		ExcursionSigmas:   3,
		ExcursionHorizon:  32,
		RetraceFraction:   0.5,
		RegimeWindow:      32,
		RegimeBaseline:    256,
		VolatilityRatio:   2.5,
		SpreadRatio:       3,
		DepthRatio:        0.25,
		ArrivalRatio:      3,
		MinRegimeSpan:     8,
		MinObservations:   64,
		MaxEpisodesPerSet: 256,
	}
}

/*
normalised fills any unset field of a declared policy from the default, so a
caller may declare only the thresholds it cares about without silently getting
a zero threshold that would qualify everything.
*/
func (policy DiscoveryPolicy) normalised() DiscoveryPolicy {
	fallback := DefaultDiscoveryPolicy()

	if !policy.Coordinate.Valid() {
		policy.Coordinate = fallback.Coordinate
	}

	if policy.FloorExcursion <= 0 {
		policy.FloorExcursion = fallback.FloorExcursion
	}

	if policy.ExcursionSigmas <= 0 {
		policy.ExcursionSigmas = fallback.ExcursionSigmas
	}

	if policy.ExcursionHorizon <= 0 {
		policy.ExcursionHorizon = fallback.ExcursionHorizon
	}

	if policy.RetraceFraction <= 0 || policy.RetraceFraction >= 1 {
		policy.RetraceFraction = fallback.RetraceFraction
	}

	if policy.RegimeWindow <= 1 {
		policy.RegimeWindow = fallback.RegimeWindow
	}

	if policy.RegimeBaseline <= policy.RegimeWindow {
		policy.RegimeBaseline = fallback.RegimeBaseline
	}

	if policy.VolatilityRatio <= 1 {
		policy.VolatilityRatio = fallback.VolatilityRatio
	}

	if policy.SpreadRatio <= 1 {
		policy.SpreadRatio = fallback.SpreadRatio
	}

	if policy.DepthRatio <= 0 || policy.DepthRatio >= 1 {
		policy.DepthRatio = fallback.DepthRatio
	}

	if policy.ArrivalRatio <= 1 {
		policy.ArrivalRatio = fallback.ArrivalRatio
	}

	if policy.MinRegimeSpan <= 0 {
		policy.MinRegimeSpan = fallback.MinRegimeSpan
	}

	if policy.MinObservations <= 0 {
		policy.MinObservations = fallback.MinObservations
	}

	if policy.MaxEpisodesPerSet <= 0 {
		policy.MaxEpisodesPerSet = fallback.MaxEpisodesPerSet
	}

	return policy
}

/*
Discovery is the result of running one declared selector over one symbol's
captured observations: the episodes found, the selector that found them, and
the explicit accounting of what could not be measured. Undefined observations
are reported as undefined rather than folded into the series as zero (§43).
*/
type Discovery struct {
	Symbol           string           `json:"symbol"`
	Coordinate       MarketCoordinate `json:"coordinate"`
	Policy           DiscoveryPolicy  `json:"policy"`
	Observations     int              `json:"observations"`
	Defined          int              `json:"defined"`
	Undefined        int              `json:"undefined"`
	Sigma            float64          `json:"sigma"`
	HasSigma         bool             `json:"hasSigma"`
	QualifyingMove   float64          `json:"qualifyingMove"`
	Episodes         []Episode        `json:"episodes"`
	InsufficientData bool             `json:"insufficientData"`
}

/*
DiscoverEpisodes runs the declared selector over one symbol's captured
observations, in CaptureSequence order, and returns every region that qualified.

Discovery may look at the whole series, including observations after a
reference point: the future is permitted to tell Hindsight where to look
(§31). It may never be fed back into the system snapshot assembled at that
reference point, and nothing in this function reads a SYMM artifact (§27).

The caller supplies observations already filtered to one symbol; they are
sorted here by CaptureSequence, never by venue or receive time (§52).
*/
func DiscoverEpisodes(symbol string, observations []Observation, policy DiscoveryPolicy) Discovery {
	policy = policy.normalised()

	discovery := Discovery{
		Symbol:       symbol,
		Coordinate:   policy.Coordinate,
		Policy:       policy,
		Observations: len(observations),
		Episodes:     make([]Episode, 0),
	}

	ordered := make([]Observation, len(observations))
	copy(ordered, observations)
	sort.SliceStable(ordered, func(left, right int) bool {
		if ordered[left].Capture.Sequence != ordered[right].Capture.Sequence {
			return ordered[left].Capture.Sequence < ordered[right].Capture.Sequence
		}

		return ordered[left].Ordinal < ordered[right].Ordinal
	})

	series := make([]sample, 0, len(ordered))

	for _, observation := range ordered {
		value, defined := observation.Value(policy.Coordinate)

		if !defined {
			discovery.Undefined++
			continue
		}

		series = append(series, sample{observation: observation, value: value})
	}

	discovery.Defined = len(series)

	if len(series) < policy.MinObservations {
		discovery.InsufficientData = true
		return discovery
	}

	sigma, hasSigma := logReturnSigma(series)
	discovery.Sigma = sigma
	discovery.HasSigma = hasSigma

	qualifying := policy.FloorExcursion

	if hasSigma {
		derived := policy.ExcursionSigmas * sigma * math.Sqrt(float64(policy.ExcursionHorizon))

		if derived > qualifying {
			qualifying = derived
		}
	}

	discovery.QualifyingMove = qualifying

	episodes := make([]Episode, 0)
	episodes = append(episodes, excursionEpisodes(symbol, series, policy, qualifying)...)
	episodes = append(episodes, volatilityEpisodes(symbol, series, policy)...)
	episodes = append(episodes, spreadEpisodes(symbol, series, policy)...)
	episodes = append(episodes, depthEpisodes(symbol, series, policy)...)
	episodes = append(episodes, arrivalEpisodes(symbol, series, policy)...)

	sort.SliceStable(episodes, func(left, right int) bool {
		if episodes[left].FromSequence != episodes[right].FromSequence {
			return episodes[left].FromSequence < episodes[right].FromSequence
		}

		return episodes[left].Kind < episodes[right].Kind
	})

	if len(episodes) > policy.MaxEpisodesPerSet {
		episodes = largestByMagnitude(episodes, policy.MaxEpisodesPerSet)
	}

	discovery.Episodes = episodes

	return discovery
}

/*
sample is one defined coordinate value paired with the observation that carried
it, so every emitted reference point can name its exact capture identity.
*/
type sample struct {
	observation Observation
	value       float64
}

/*
excursionEpisodes walks the coordinate series and confirms directional legs by
retracement: the leg running from the last pivot to the running extremum is
confirmed once the coordinate retraces RetraceFraction of the qualifying move
away from that extremum. Confirmation is what makes the geometry objective — a
leg is never declared by choosing whichever endpoint best explains a later
event (§58).

The final leg of a tape has, by construction, no retracement after it. It is
still emitted when it qualifies, marked Confirmed=false, because refusing to
show the most recent excursion of a run still being captured would hide market
evidence; claiming it as confirmed geometry would state more than the record
supports (§33).

Two consecutive qualifying legs of opposite sign additionally emit a Reversal
episode spanning both, whose Reversal reference is their shared pivot.
*/
func excursionEpisodes(
	symbol string,
	series []sample,
	policy DiscoveryPolicy,
	qualifying float64,
) []Episode {
	confirm := qualifying * policy.RetraceFraction

	if confirm <= 0 {
		return nil
	}

	legs := make([]leg, 0)

	pivot := 0
	extremum := 0
	rising := true

	for index := 1; index < len(series); index++ {
		value := series[index].value

		if series[extremum].value <= 0 {
			extremum = index
			continue
		}

		if rising {
			if value >= series[extremum].value {
				extremum = index
				continue
			}

			if (series[extremum].value-value)/series[extremum].value < confirm {
				continue
			}
		} else {
			if value <= series[extremum].value {
				extremum = index
				continue
			}

			if (value-series[extremum].value)/series[extremum].value < confirm {
				continue
			}
		}

		if extremum != pivot {
			legs = append(legs, leg{from: pivot, to: extremum, confirmed: true})
		}

		pivot = extremum
		extremum = index
		rising = !rising
	}

	if extremum > pivot {
		legs = append(legs, leg{from: pivot, to: extremum, confirmed: false})
	}

	episodes := make([]Episode, 0, len(legs))
	qualified := make([]leg, 0, len(legs))

	for _, current := range legs {
		origin := series[current.from].value

		if origin <= 0 {
			continue
		}

		excursion := (series[current.to].value - origin) / origin

		if math.Abs(excursion) < qualifying {
			continue
		}

		current.excursion = excursion
		qualified = append(qualified, current)

		kind := EpisodeUpwardExcursion
		endpoint := ReferencePeak

		if excursion < 0 {
			kind = EpisodeDownwardExcursion
			endpoint = ReferenceTrough
		}

		episode := newEpisode(symbol, kind, policy.Coordinate, series, current.from, current.to)
		episode.ObservedExcursion = excursion
		episode.HasObservedExcursion = true
		episode.Confirmed = current.confirmed
		episode.References = []ReferencePoint{
			referenceAt(ReferenceAnchor, series[current.from]),
			referenceAt(endpoint, series[current.to]),
		}

		episodes = append(episodes, episode)
	}

	for index := 1; index < len(qualified); index++ {
		previous := qualified[index-1]
		current := qualified[index]

		if previous.to != current.from {
			continue
		}

		if (previous.excursion > 0) == (current.excursion > 0) {
			continue
		}

		origin := series[previous.from].value

		if origin <= 0 {
			continue
		}

		episode := newEpisode(
			symbol,
			EpisodeReversal,
			policy.Coordinate,
			series,
			previous.from,
			current.to,
		)
		episode.ObservedExcursion = (series[current.to].value - origin) / origin
		episode.HasObservedExcursion = true
		episode.Traversed = math.Abs(previous.excursion) + math.Abs(current.excursion)
		episode.HasTraversed = true
		episode.Confirmed = previous.confirmed && current.confirmed
		episode.References = []ReferencePoint{
			referenceAt(ReferenceAnchor, series[previous.from]),
			referenceAt(ReferenceReversal, series[previous.to]),
			referenceAt(ReferenceExitAnchor, series[current.to]),
		}

		episodes = append(episodes, episode)
	}

	return episodes
}

type leg struct {
	from      int
	to        int
	excursion float64
	confirmed bool
}

/*
volatilityEpisodes compares the realised dispersion of log returns over the
policy's short window against the same statistic over its longer baseline, and
marks the spans where that ratio sustained an expansion or a contraction.
*/
func volatilityEpisodes(symbol string, series []sample, policy DiscoveryPolicy) []Episode {
	returns := make([]float64, len(series))
	defined := make([]bool, len(series))

	for index := 1; index < len(series); index++ {
		previous := series[index-1].value
		current := series[index].value

		if previous <= 0 || current <= 0 {
			continue
		}

		change := math.Log(current / previous)

		if !finite(change) {
			continue
		}

		returns[index] = change
		defined[index] = true
	}

	ratios := rollingDispersionRatio(returns, defined, policy.RegimeWindow, policy.RegimeBaseline)

	expansion := regimeEpisodes(
		symbol,
		EpisodeVolatilityExpansion,
		policy.Coordinate,
		series,
		ratios,
		func(ratio float64) bool { return ratio >= policy.VolatilityRatio },
		policy.VolatilityRatio,
		policy.MinRegimeSpan,
		true,
	)

	contraction := regimeEpisodes(
		symbol,
		EpisodeVolatilityContraction,
		policy.Coordinate,
		series,
		ratios,
		func(ratio float64) bool { return ratio > 0 && ratio <= 1/policy.VolatilityRatio },
		1/policy.VolatilityRatio,
		policy.MinRegimeSpan,
		false,
	)

	return append(expansion, contraction...)
}

/*
spreadEpisodes marks spans where the quoted spread sustained a multiple of its
own rolling baseline. It reports what the venue quoted (§38) and never converts
that into an assumed execution cost (§35).
*/
func spreadEpisodes(symbol string, series []sample, policy DiscoveryPolicy) []Episode {
	values := make([]float64, len(series))
	defined := make([]bool, len(series))

	for index, entry := range series {
		spread, ok := entry.observation.SpreadFraction()

		if !ok {
			continue
		}

		values[index] = spread
		defined[index] = true
	}

	ratios := rollingLevelRatio(values, defined, policy.RegimeWindow, policy.RegimeBaseline)

	return regimeEpisodes(
		symbol,
		EpisodeSpreadExpansion,
		policy.Coordinate,
		series,
		ratios,
		func(ratio float64) bool { return ratio >= policy.SpreadRatio },
		policy.SpreadRatio,
		policy.MinRegimeSpan,
		true,
	)
}

/*
depthEpisodes marks spans where the size quoted at the touch collapsed to a
fraction of its own rolling baseline. Quoted size is not executable quantity
(§34, §36); this is an observation about the visible book only.
*/
func depthEpisodes(symbol string, series []sample, policy DiscoveryPolicy) []Episode {
	values := make([]float64, len(series))
	defined := make([]bool, len(series))

	for index, entry := range series {
		depth, ok := entry.observation.TouchDepth()

		if !ok {
			continue
		}

		values[index] = depth
		defined[index] = true
	}

	ratios := rollingLevelRatio(values, defined, policy.RegimeWindow, policy.RegimeBaseline)

	return regimeEpisodes(
		symbol,
		EpisodeLiquidityCollapse,
		policy.Coordinate,
		series,
		ratios,
		func(ratio float64) bool { return ratio > 0 && ratio <= policy.DepthRatio },
		policy.DepthRatio,
		policy.MinRegimeSpan,
		false,
	)
}

/*
arrivalEpisodes marks spans where observations arrived at a multiple of their
own baseline rate. The rate is measured on the venue/receive instants the
observations actually carry, and the span is still addressed by CaptureSequence
(§8, §9).
*/
func arrivalEpisodes(symbol string, series []sample, policy DiscoveryPolicy) []Episode {
	gaps := make([]float64, len(series))
	defined := make([]bool, len(series))

	for index := 1; index < len(series); index++ {
		previous := series[index-1].observation.At()
		current := series[index].observation.At()

		if previous.IsZero() || current.IsZero() {
			continue
		}

		elapsed := current.Sub(previous).Seconds()

		if elapsed <= 0 || !finite(elapsed) {
			continue
		}

		gaps[index] = elapsed
		defined[index] = true
	}

	intervals := rollingLevelRatio(gaps, defined, policy.RegimeWindow, policy.RegimeBaseline)
	rates := make([]regimeRatio, len(intervals))

	for index, interval := range intervals {
		if !interval.defined || interval.ratio <= 0 {
			continue
		}

		rates[index] = regimeRatio{ratio: 1 / interval.ratio, defined: true}
	}

	return regimeEpisodes(
		symbol,
		EpisodeArrivalCluster,
		policy.Coordinate,
		series,
		rates,
		func(ratio float64) bool { return ratio >= policy.ArrivalRatio },
		policy.ArrivalRatio,
		policy.MinRegimeSpan,
		true,
	)
}

type regimeRatio struct {
	ratio   float64
	defined bool
}

/*
regimeEpisodes turns a ratio series and a qualifying predicate into contiguous
spans of at least minSpan qualifying observations. The span's ShockOnset is its
first qualifying observation — the point the regime was first observed, never
the point that best explains a later move (§58). extreme selects whether the
episode's reported ratio is the span's maximum or its minimum.
*/
func regimeEpisodes(
	symbol string,
	kind EpisodeKind,
	coordinate MarketCoordinate,
	series []sample,
	ratios []regimeRatio,
	qualifies func(float64) bool,
	threshold float64,
	minSpan int,
	upper bool,
) []Episode {
	episodes := make([]Episode, 0)

	start := -1
	extreme := 0.0

	flush := func(end int, confirmed bool) {
		if start < 0 {
			return
		}

		if end-start+1 >= minSpan {
			episode := newEpisode(symbol, kind, coordinate, series, start, end)
			episode.Ratio = extreme
			episode.HasRatio = true
			episode.Threshold = threshold
			episode.HasThreshold = threshold > 0
			episode.Confirmed = confirmed
			episode.References = []ReferencePoint{
				referenceAt(ReferenceShockOnset, series[start]),
			}

			episodes = append(episodes, episode)
		}

		start = -1
	}

	for index, entry := range ratios {
		if !entry.defined || !qualifies(entry.ratio) {
			flush(index-1, true)
			continue
		}

		if start < 0 {
			start = index
			extreme = entry.ratio
		}

		if upper && entry.ratio > extreme {
			extreme = entry.ratio
		}

		if !upper && entry.ratio < extreme {
			extreme = entry.ratio
		}
	}

	flush(len(ratios)-1, false)

	return episodes
}

/*
rollingDispersionRatio returns, per index, the standard deviation of the last
window defined values over the standard deviation of the last baseline defined
values. Both statistics are computed only from defined entries; an undefined
entry is skipped, never read as a zero return (§43).
*/
func rollingDispersionRatio(values []float64, defined []bool, window, baseline int) []regimeRatio {
	ratios := make([]regimeRatio, len(values))

	for index := range values {
		short, shortOK := dispersionOver(values, defined, index, window)
		long, longOK := dispersionOver(values, defined, index, baseline)

		if !shortOK || !longOK || long <= 0 {
			continue
		}

		ratio := short / long

		if !finite(ratio) {
			continue
		}

		ratios[index] = regimeRatio{ratio: ratio, defined: true}
	}

	return ratios
}

/*
rollingLevelRatio returns, per index, the mean of the last window defined
values over the mean of the last baseline defined values.
*/
func rollingLevelRatio(values []float64, defined []bool, window, baseline int) []regimeRatio {
	ratios := make([]regimeRatio, len(values))

	for index := range values {
		short, shortOK := meanOver(values, defined, index, window)
		long, longOK := meanOver(values, defined, index, baseline)

		if !shortOK || !longOK || long <= 0 {
			continue
		}

		ratio := short / long

		if !finite(ratio) {
			continue
		}

		ratios[index] = regimeRatio{ratio: ratio, defined: true}
	}

	return ratios
}

/*
meanOver averages the defined values in the span ending at index and covering
span entries, requiring at least half the span to be defined before it answers.
Insufficient support is answered as undefined, never as an estimate (§40).
*/
func meanOver(values []float64, defined []bool, index, span int) (float64, bool) {
	start := index - span + 1

	if start < 0 {
		return 0, false
	}

	total := 0.0
	count := 0

	for cursor := start; cursor <= index; cursor++ {
		if !defined[cursor] {
			continue
		}

		total += values[cursor]
		count++
	}

	if count*2 < span {
		return 0, false
	}

	return total / float64(count), true
}

/*
dispersionOver is the population standard deviation of the defined values in
the span ending at index, with the same support requirement as meanOver.
*/
func dispersionOver(values []float64, defined []bool, index, span int) (float64, bool) {
	mean, ok := meanOver(values, defined, index, span)

	if !ok {
		return 0, false
	}

	start := index - span + 1
	total := 0.0
	count := 0

	for cursor := start; cursor <= index; cursor++ {
		if !defined[cursor] {
			continue
		}

		delta := values[cursor] - mean
		total += delta * delta
		count++
	}

	if count < 2 {
		return 0, false
	}

	return math.Sqrt(total / float64(count)), true
}

/*
logReturnSigma is the population standard deviation of the series' consecutive
log returns. It is the symbol's own dispersion, used to derive the qualifying
excursion rather than assume one.
*/
func logReturnSigma(series []sample) (float64, bool) {
	returns := make([]float64, 0, len(series))

	for index := 1; index < len(series); index++ {
		previous := series[index-1].value
		current := series[index].value

		if previous <= 0 || current <= 0 {
			continue
		}

		change := math.Log(current / previous)

		if finite(change) {
			returns = append(returns, change)
		}
	}

	if len(returns) < 2 {
		return 0, false
	}

	mean := 0.0

	for _, value := range returns {
		mean += value
	}

	mean /= float64(len(returns))

	total := 0.0

	for _, value := range returns {
		delta := value - mean
		total += delta * delta
	}

	sigma := math.Sqrt(total / float64(len(returns)))

	return sigma, finite(sigma) && sigma > 0
}

/*
newEpisode builds the episode shell spanning series[from..to], addressed by the
capture identities at both ends.
*/
func newEpisode(
	symbol string,
	kind EpisodeKind,
	coordinate MarketCoordinate,
	series []sample,
	from, to int,
) Episode {
	first := series[from].observation
	last := series[to].observation

	return Episode{
		ID: fmt.Sprintf(
			"%s|%s|%d|%d",
			kind,
			symbol,
			first.Capture.Sequence,
			last.Capture.Sequence,
		),
		Symbol:       symbol,
		Kind:         kind,
		Coordinate:   coordinate,
		FromSequence: first.Capture.Sequence,
		ToSequence:   last.Capture.Sequence,
		FromAt:       first.At(),
		ToAt:         last.At(),
		Observations: to - from + 1,
		References:   make([]ReferencePoint, 0, 3),
	}
}

func referenceAt(role ReferenceRole, entry sample) ReferencePoint {
	return ReferencePoint{
		Role:       role,
		Capture:    entry.observation.Capture,
		Ordinal:    entry.observation.Ordinal,
		VenueAt:    entry.observation.VenueAt,
		ReceivedAt: entry.observation.ReceivedAt,
		Value:      entry.value,
		HasValue:   true,
	}
}

/*
largestByMagnitude bounds the retained set without letting one family of
evidence crowd out another: price geometry is kept first, ranked by the
distance the coordinate travelled, and the remaining room goes to the largest
regime spans by their own exceedance. Capture order is then restored, so the
surviving set is still a causally ordered tape (§52).
*/
func largestByMagnitude(episodes []Episode, limit int) []Episode {
	geometry := make([]Episode, 0, len(episodes))
	regimes := make([]Episode, 0, len(episodes))

	for _, episode := range episodes {
		if episode.IsPriceGeometry() {
			geometry = append(geometry, episode)
			continue
		}

		regimes = append(regimes, episode)
	}

	byMagnitude := func(set []Episode) {
		sort.SliceStable(set, func(left, right int) bool {
			return set[left].Magnitude() > set[right].Magnitude()
		})
	}

	byMagnitude(geometry)
	byMagnitude(regimes)

	if len(geometry) > limit {
		geometry = geometry[:limit]
	}

	kept := geometry
	room := limit - len(kept)

	if room > len(regimes) {
		room = len(regimes)
	}

	if room > 0 {
		kept = append(kept, regimes[:room]...)
	}

	sort.SliceStable(kept, func(left, right int) bool {
		if kept[left].FromSequence != kept[right].FromSequence {
			return kept[left].FromSequence < kept[right].FromSequence
		}

		return kept[left].Kind < kept[right].Kind
	})

	return kept
}
