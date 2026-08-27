package types

import "time"

/*
PerspectiveKind names the family of descriptive context a Perspective carries.
It is an interned numeric identity, not a rendered string: the hot path compares
and keys on the integer, never on a string built for display.
*/
type PerspectiveKind uint8

const (
	// KindHistoricalAnalogue describes how closely the current symbol trajectory
	// resembles previously archived episodes of the same symbol's own history.
	KindHistoricalAnalogue PerspectiveKind = iota + 1
	// KindRelativeState describes how the current symbol's regime compares with
	// the rest of the market population's regimes.
	KindRelativeState
)

/*
String returns the display name of a kind. It is for telemetry and UI rendering
only and must never participate in a hot-path comparison or key.
*/
func (kind PerspectiveKind) String() string {
	switch kind {
	case KindHistoricalAnalogue:
		return "historical_analogue"
	case KindRelativeState:
		return "relative_state"
	default:
		return "unknown"
	}
}

/*
HistoricalAnaloguePayload is the fixed-size descriptive payload of a
HistoricalAnalogue Perspective. Definedness is carried by Support: a Support of
zero means the advisor has not yet archived any comparable episode, and the
distance fields are then meaningless — they are never fabricated zeros standing
in for a real estimate.
*/
type HistoricalAnaloguePayload struct {
	/*
		Support is how many completed historical windows the archive currently
		holds. Zero means no comparison was possible.
	*/
	Support int

	/*
		NearestDistance is the minimum normalized Hamming distance between the
		current trajectory and any archived window, in [0,1]. Defined only when
		Support >= 1.
	*/
	NearestDistance float64

	/*
		MedianDistance is the median normalized Hamming distance from the
		current trajectory across the archive. It is the symbol's own typical
		self-distance, supplied as the honest scale against which a consumer
		judges whether NearestDistance is unusually close — without a tuned
		threshold. Defined only when Support >= 1.
	*/
	MedianDistance float64

	/*
		StageAlignment is the current trajectory's fill fraction, in [0,1]:
		how far along the declared window the in-progress episode has advanced.
		It answers "where in the episode is now" without estimating the matched
		episode's duration.
	*/
	StageAlignment float64
}

/*
RelativeStatePayload is the fixed-size descriptive payload of a RelativeState
Perspective: how the symbol's current regime sits within the market population.
Definedness is carried by PeerCount; a PeerCount of zero means no population
exists to compare against, and Breadth is then meaningless.
*/
type RelativeStatePayload struct {
	/*
		PeerCount is how many symbols currently hold a resolved dominant regime
		(including this one). Zero means no comparison was possible.
	*/
	PeerCount int

	/*
		SameRegime is how many of those peers currently share this symbol's
		dominant regime.
	*/
	SameRegime int

	/*
		Breadth is SameRegime / PeerCount, the fraction of the population
		currently in this symbol's regime, in [0,1]. A small breadth means the
		symbol is behaving idiosyncratically relative to the population; it is
		a measured share, not an outlier score.
	*/
	Breadth float64

	/*
		MajorityRegime is the most frequent regime across the population,
		interned through the category vocabulary. Zero when no regime resolved.
	*/
	MajorityRegime uint8

	/*
		MajorityBreadth is the majority regime's share of the population, in
		[0,1].
	*/
	MajorityBreadth float64
}

/*
Perspective is the current descriptive output of one Advisor. It is context,
never an instruction: a Perspective describes the past and present and does not
choose an action, impose a gate, or assert an opportunity score.

The envelope is fixed-size apart from the two symbol identities; the
kind-specific numeric payloads are value structs, so emitting a Perspective
allocates no per-event slices or maps.
*/
type Perspective struct {
	Symbol string
	/*
		Peer is the counterpart symbol for a relationship-kind perspective and
		is empty for symbol-local and global perspectives.
	*/
	Peer string

	Kind PerspectiveKind

	/*
		From is the earliest observation folded into the current trajectory and
		At is the latest; together they bound the context window.
	*/
	From     time.Time
	At       time.Time
	Sequence uint64
	/*
		Maturity is the perspective's own readiness, in [0,1]. It is a bounded
		descriptive quantity (for HistoricalAnalogue, the trajectory fill
		fraction), never a generic confidence scalar.
	*/
	Maturity float64

	/*
		Analogue is the HistoricalAnalogue payload. It is meaningful only when
		Kind is KindHistoricalAnalogue.
	*/
	Analogue HistoricalAnaloguePayload

	/*
		Relative is the RelativeState payload. It is meaningful only when Kind
		is KindRelativeState.
	*/
	Relative RelativeStatePayload
}

/*
PerspectiveKey is the structural identity of one perspective: the symbol (and
optional peer) plus the kind. It is a comparable value type so it can be used
directly as a map key and compared without allocating or building strings. A
PositionID- or global-scoped perspective is a later extension of this key.
*/
type PerspectiveKey struct {
	Symbol string
	Peer   string
	Kind   PerspectiveKind
}

/*
Key returns the structural identity of the perspective.
*/
func (perspective Perspective) Key() PerspectiveKey {
	return PerspectiveKey{
		Symbol: perspective.Symbol,
		Peer:   perspective.Peer,
		Kind:   perspective.Kind,
	}
}
