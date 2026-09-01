package types

import (
	"sort"
	"sync"
	"time"

	flatbuffers "github.com/google/flatbuffers/go"
	"github.com/theapemachine/symm/hindsight"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/learning"
	"github.com/theapemachine/symm/nomagique/physics/sensorium"
	nmtypes "github.com/theapemachine/symm/nomagique/types"
	"github.com/theapemachine/symm/telemetry/generated/telemetry"
)

/*
GraphUpdate is the domain event the graph stage publishes after each
Measurement advances the Influence Graph. It carries only the symbol and
as-of time; consumers read the shared Influence Graph for the data itself.
*/
type GraphUpdate struct {
	Symbol string
	At     time.Time
}

/*
WaveMode is one resident ω-lattice spectral mode: its lattice frequency,
complex spectral-head coefficient, and gate linewidth.
*/
type WaveMode struct {
	Omega     float32
	Real      float32
	Imag      float32
	Linewidth float32
}

/*
ManifoldState is everything the manifold stage produces for one advance of
the resident domain: the per-particle sensorium.State and Reading, the packed
Eulerian gas/wave grid fields, and the resident spectral mode lattice.
*/
type ManifoldState struct {
	sensorium.State
	sensorium.Reading

	GridX, GridY, GridZ int
	GridSpacing         float64

	// MomRho is four floats per grid cell: momentum xyz then density.
	MomRho []float32
	// FieldEnergy is the packed Eulerian energy field, one value per grid
	// cell — distinct from the embedded sensorium.State.Energy, which is
	// per-particle.
	FieldEnergy []float32
	// WaveReal and WaveImag are the complex spatial wave field, one value per
	// grid cell.
	WaveReal []float32
	WaveImag []float32
	// DensityScale, MomentumScale, EnergyScale, and WaveScale are the peak
	// magnitudes PackFields observed while packing MomRho/FieldEnergy/
	// WaveReal/WaveImag, for normalizing a renderer's color/volume mapping.
	DensityScale  float32
	MomentumScale float32
	EnergyScale   float32
	WaveScale     float32

	Modes []WaveMode
}

/*
BoundaryStamp is one diagnostics node's "the envelope passed here" witness:
the node's own label, when it observed the envelope, and a running summary of
how often and how regularly this exact stage runs (see system.Diagnostic).
Every diagnostics node in a Workload's declared stage list appends its own
stamp as the envelope passes, so Boundaries ends up as an ordered trace of
every named stage boundary the envelope crossed. That trace is enough on its
own for a consumer to derive topology (consecutive labels are an edge),
per-hop latency (the AtNs delta between consecutive stamps), and per-stage
rate/health (SeqCount/AvgGapNs/LastGapNs) — no hand-maintained graph.
*/
type BoundaryStamp struct {
	Label string
	AtNs  int64

	// SeqCount is this stage's lifetime call count.
	SeqCount int64

	// AvgGapNs is an EMA of the time between consecutive calls to this exact
	// stage — a smoothed read on its throughput independent of any single
	// envelope's jitter.
	AvgGapNs int64

	// LastGapNs is the time since this stage's immediately preceding call,
	// unsmoothed — the freshest possible reading of its current pace.
	LastGapNs int64

	// Backlog is how many sequence numbers behind the Workload's producer
	// this envelope already was when this stage reached it — real ring
	// pressure read from the same sequence numbers the disruptor itself uses
	// for backpressure (see runtime.BacklogStepper), not estimated from
	// rates. 0 means this stage is fully caught up with the producer.
	Backlog int64

	// Group is the name of the runtime.Workload ring this stage runs in, and
	// Stage its handler-group index within that ring. The Workload stamps
	// both onto its own nodes as it composes them, so the trace carries the
	// ring structure the envelope actually crossed rather than only the order
	// its labels happened to appear. That distinction matters because the
	// nodes of one handler group run concurrently: their labels arrive in
	// goroutine-completion order, which reads as a chain of hops between
	// siblings that never actually feed each other.
	Group string
	Stage int32
}

type TypeID uint8

const (
	EnvelopeUnknown TypeID = iota
	EnvelopeTicker
	EnvelopeTrade
	EnvelopeLevel3
	EnvelopeExecution
	EnvelopeFuturesTicker
	EnvelopeFuturesTrade
	EnvelopeCorrelation
	EnvelopeCVD
	EnvelopeDepthFlow
	EnvelopeMorphology
	EnvelopePumpDump
	EnvelopeToxicity
	EnvelopeDerivatives
	EnvelopeLeadlag
	EnvelopeLiquidity
	EnvelopeResonance
	EnvelopeManifold
	EnvelopeGraph
	EnvelopeCausal
	EnvelopeCategory
	EnvelopeCognition
	EnvelopeAdvisor
	EnvelopeMCTS
)

type Envelope struct {
	Key    string
	TypeID TypeID

	// Tick is the engine clock at which this envelope was produced: the
	// monotonic thesis counter, stamped when the ingress observation commits.
	// It is the "tick counter" the dashboard renders, distinct from the
	// per-ticker wall-clock Timestamp carried in TickerData.
	Tick int64

	// CaptureID is the Hindsight capture identity of the exact raw external
	// input this envelope was parsed from. It is assigned before parsing and
	// carried unchanged for the envelope's whole ring traversal; a zero value
	// means the ingress stream was not wired with a capture sequencer.
	CaptureID hindsight.CaptureIdentity

	// CaptureOrdinal is this envelope's deterministic ordinal within the raw
	// frame that produced it (§12). A single raw frame may yield zero, one, or
	// many envelopes; the ordinal disambiguates them in parser order.
	CaptureOrdinal uint64

	// Stream is the operational transport identity of the exact raw external
	// input this envelope was parsed from. It is minted and advanced by the
	// websocket transport (epoch bumps on reconnect, sequence on frame) and is
	// present whether or not Hindsight capture is enabled. Live trading reads
	// this operational metadata; Hindsight records the same fact in CaptureID.
	Stream hindsight.StreamRef

	TickerData        kraken.TickerData
	TradeData         kraken.TradeData
	Level3Data        kraken.Level3Data
	ExecutionData     kraken.ExecutionData
	FuturesTickerData kraken.FuturesTickerData
	FuturesTradeData  kraken.FuturesTradeData

	// Signal outputs, one field per signal so concurrent Nodes in the same
	// disruptor HandlerGroup each own a distinct field and never race on a
	// shared slice header or index.
	Correlation *data.Measurement[float64]
	LeadLag     *data.Measurement[float64]
	Liquidity   *data.Measurement[float64]
	Sentiment   *data.Measurement[float64]
	CVD         *data.Measurement[float64]
	DepthFlow   *data.Measurement[float64]
	Morphology  *data.Measurement[float64]
	Hawkes      *data.Measurement[float64]
	PumpDump    *data.Measurement[float64]
	Toxicity    *data.Measurement[float64]
	Derivatives *data.Measurement[float64]

	// Categories is the ranked regime batch the category stage resolves from
	// whichever signal outputs above are populated on this envelope.
	Categories []Category

	// Opportunities is the opportunity stage's tracked candidates, resolved
	// from Categories.
	Opportunities []*OpportunityCandidate

	// GraphUpdate is the graph stage's last domain event for this envelope.
	GraphUpdate *GraphUpdate

	// Resonance is the resonance stage's predictive-coder artifact, derived
	// from TickerData.
	Resonance *ResonanceArtifact

	// Manifold is the manifold stage's resident-field advance, produced when
	// a Trade envelope carries a Hawkes measurement (the forcing term).
	Manifold *ManifoldState

	// Cognition is the cognition stage's freshest reading, resolved from
	// Categories.
	Cognition *Cognition

	// StrategyRound is the strategy stage's last decision round, produced by
	// the planner once per engine tick and stamped by tickNode so the same
	// envelope that carried the logic inputs carries the decisions it produced.
	StrategyRound *StrategyRound

	// Equity is the account valuation as of this envelope: cash, unrealized
	// profit/loss, and total equity, read from the thesis the broker desk
	// publishes into. It is stamped on ticker envelopes alongside the engine
	// tick, so a dashboard that connects (or reconnects) mid-session recovers
	// the balance from the next market event rather than never at all.
	Equity *EquityReading

	// Positions is the desk's open-lot snapshot as of this envelope, one
	// wire.Position per open lot (closed lots excluded), projected into a
	// PositionsFrame. It rides ticker envelopes exactly like equity, so the
	// positions panel recovers on the next market event after a connect rather
	// than waiting for a poll.
	Positions []*telemetry.PositionT

	// Perspectives is the advisory layer's descriptive context: one
	// Perspective per advisor family, composed from this envelope's signal
	// measurements. Perspectives describe the current state; they are never
	// decisions, gates, or scores, and are consumed by decision and risk.
	Perspectives []*Perspective

	// Boundaries is the ordered trace of every diagnostics node the envelope
	// passed through, appended to via AppendBoundary as it crosses each one
	// (see system.Diagnostic). Growable rather than fixed-size: stage count
	// differs per workload and a diagnostics node has no way to know its
	// ordinal position ahead of time. Unlike every other field above,
	// concurrent Nodes in the same HandlerGroup CAN all write here (a
	// per-signal Diagnostic wrapping each signal in a concurrent stage) —
	// boundariesMu is the one exception to this type's lock-free discipline,
	// guarding only the append itself.
	boundariesMu sync.Mutex
	Boundaries   []BoundaryStamp
}

/*
AppendBoundary adds one diagnostics stamp to Boundaries under boundariesMu.
The lock is held only across the append itself (no I/O, no allocation beyond
the occasional slice growth), so contention is bounded by how many concurrent
diagnostics nodes touch one envelope in a single stage (a handful, from the
signal fan-out) — never by the number of symbols or envelopes in flight,
since each envelope has its own independent Envelope and mutex.
*/
func (envelope *Envelope) AppendBoundary(stamp BoundaryStamp) {
	envelope.boundariesMu.Lock()
	envelope.Boundaries = append(envelope.Boundaries, stamp)
	envelope.boundariesMu.Unlock()
}

func NewEnvelope(typeID TypeID) *Envelope {
	return &Envelope{
		TypeID: typeID,
	}
}

/*
SignalMeasurements returns the eleven canonical signal measurement fields in
their fixed declaration order, so a consumer that folds "every populated signal
measurement" iterates ONE authoritative list instead of re-enumerating the
fields by hand. A new signal family added as an Envelope field is only ever a
one-line change here — it can no longer be silently stranded because some
downstream consumer's hard-coded slice was not updated.

The return is a fixed-size array (not a slice), so the call allocates nothing
and each consumer iterates the same canonical order. Nil entries mean that
signal did not fire on this envelope.
*/
func (envelope *Envelope) SignalMeasurements() [11]*data.Measurement[float64] {
	if envelope == nil {
		return [11]*data.Measurement[float64]{}
	}

	return [11]*data.Measurement[float64]{
		envelope.Correlation,
		envelope.LeadLag,
		envelope.Liquidity,
		envelope.Sentiment,
		envelope.CVD,
		envelope.DepthFlow,
		envelope.Morphology,
		envelope.Hawkes,
		envelope.PumpDump,
		envelope.Toxicity,
		envelope.Derivatives,
	}
}

func timeNs(at time.Time) int64 {
	if at.IsZero() {
		return 0
	}

	return at.UnixNano()
}

func encodeMeasurement(measurement *data.Measurement[float64]) *telemetry.EnvelopeMeasurementT {
	if measurement == nil {
		return nil
	}

	metrics := make([]*telemetry.EnvelopeMeasurementMetricT, 0, len(measurement.Metrics))

	for key, metric := range measurement.Metrics {
		encoded := &telemetry.EnvelopeMetricT{
			Label:     metric.Label,
			Raw:       metric.Raw,
			Unit:      string(metric.Unit),
			Timescale: string(metric.Timescale),
		}

		if metric.Normalized != nil {
			encoded.HasNormalized = true
			encoded.Normalized = *metric.Normalized
		}

		if metric.Standardized != nil {
			encoded.HasStandardized = true
			encoded.Standardized = *metric.Standardized
		}

		metrics = append(metrics, &telemetry.EnvelopeMeasurementMetricT{Key: key, Value: encoded})
	}

	metadata := make([]*telemetry.NamedNumberT, 0, len(measurement.Metadata))

	for key, value := range measurement.Metadata {
		metadata = append(metadata, &telemetry.NamedNumberT{Name: key, Value: value})
	}

	provenance := make([]*telemetry.NamedStringT, 0, len(measurement.Provenance))

	for key, value := range measurement.Provenance {
		provenance = append(provenance, &telemetry.NamedStringT{Name: key, Value: value})
	}

	return &telemetry.EnvelopeMeasurementT{
		Id:         measurement.ID,
		Label:      measurement.Label,
		Source:     measurement.Source,
		SeqIdx:     measurement.SeqIdx,
		AtNs:       timeNs(measurement.At),
		HasFrom:    !measurement.From.IsZero(),
		FromNs:     timeNs(measurement.From),
		Maturity:   measurement.Maturity,
		Snr:        measurement.SNR,
		SnrDefined: measurement.SNRDefined,
		Metrics:    metrics,
		Metadata:   metadata,
		Provenance: provenance,
	}
}

func encodeCategories(categories []Category) []*telemetry.EnvelopeCategoryT {
	if categories == nil {
		return nil
	}

	encoded := make([]*telemetry.EnvelopeCategoryT, len(categories))

	for index, category := range categories {
		encoded[index] = &telemetry.EnvelopeCategoryT{
			AtNs:        timeNs(category.At),
			Symbol:      category.Symbol,
			Type:        string(category.Type),
			Confidence:  category.Confidence,
			Surprisal:   category.Surprisal,
			Strength:    category.Strength,
			Maturity:    category.Maturity,
			Uncertainty: category.Uncertainty,
			Freshness:   category.Freshness,
			Supporting:  category.Supporting,
			Opposing:    category.Opposing,
			Missing:     category.Missing,
		}
	}

	return encoded
}

func encodeOpportunities(opportunities []*OpportunityCandidate) []*telemetry.EnvelopeOpportunityCandidateT {
	if opportunities == nil {
		return nil
	}

	encoded := make([]*telemetry.EnvelopeOpportunityCandidateT, 0, len(opportunities))

	for _, candidate := range opportunities {
		if candidate == nil {
			continue
		}

		out := &telemetry.EnvelopeOpportunityCandidateT{
			Symbol:      candidate.Symbol,
			Archetype:   string(candidate.Archetype),
			Phase:       string(candidate.Phase),
			Direction:   int32(candidate.Direction),
			FirstSeenNs: timeNs(candidate.FirstSeen),
			UpdatedNs:   timeNs(candidate.Updated),
			Sequence:    candidate.Sequence,
			Provenance:  byte(candidate.Provenance),
			Maturity:    candidate.Maturity,
		}

		if candidate.Economics != nil {
			out.Economics = &telemetry.EnvelopeOpportunityEconomicsT{
				Calibrated:            candidate.Economics.Calibrated,
				TransitionProbability: candidate.Economics.TransitionProbability,
				ProfitFirst:           candidate.Economics.ProfitFirst,
				FavorableExcursion: &telemetry.EnvelopeExcursionT{
					Low:  candidate.Economics.FavorableExcursion.Low,
					Mid:  candidate.Economics.FavorableExcursion.Mid,
					High: candidate.Economics.FavorableExcursion.High,
				},
				AdverseExcursion: &telemetry.EnvelopeExcursionT{
					Low:  candidate.Economics.AdverseExcursion.Low,
					Mid:  candidate.Economics.AdverseExcursion.Mid,
					High: candidate.Economics.AdverseExcursion.High,
				},
				ResolutionNs: int64(candidate.Economics.Resolution),
				Uncertainty:  candidate.Economics.Uncertainty,
			}
		}

		encoded = append(encoded, out)
	}

	return encoded
}

func encodeGraphUpdate(update *GraphUpdate) *telemetry.EnvelopeGraphUpdateT {
	if update == nil {
		return nil
	}

	return &telemetry.EnvelopeGraphUpdateT{Symbol: update.Symbol, AtNs: timeNs(update.At)}
}

func encodeCognition(cognition *Cognition) *telemetry.EnvelopeCognitionT {
	if cognition == nil {
		return nil
	}

	predictions := make([]*telemetry.EnvelopeCognitionPredictionT, 0, len(cognition.Predictions))

	for key, value := range cognition.Predictions {
		predictions = append(predictions, &telemetry.EnvelopeCognitionPredictionT{Key: key, Value: value})
	}

	branches := make([]*telemetry.EnvelopeCognitionBranchT, len(cognition.Branches))

	for index, branch := range cognition.Branches {
		branches[index] = &telemetry.EnvelopeCognitionBranchT{
			Id:          int64(branch.ID),
			ParentId:    int64(branch.ParentID),
			Token:       branch.Token,
			Prefix:      branch.Prefix,
			Key:         branch.Key,
			Depth:       int64(branch.Depth),
			Probability: branch.Probability,
			Count:       branch.Count,
		}
	}

	beams := make([]*telemetry.EnvelopeCognitionBeamT, len(cognition.Beams))

	for index, beam := range cognition.Beams {
		beams[index] = &telemetry.EnvelopeCognitionBeamT{Sequence: beam.Sequence, Key: beam.Key, Score: beam.Score}
	}

	classes := make([]*telemetry.EnvelopeCognitionClassT, len(cognition.Classes))

	for index, class := range cognition.Classes {
		classes[index] = &telemetry.EnvelopeCognitionClassT{Name: class.Name, Probability: class.Probability}
	}

	contributions := make([]*telemetry.EnvelopeCognitionContributionT, len(cognition.Contributions))

	for index, contribution := range cognition.Contributions {
		contributions[index] = &telemetry.EnvelopeCognitionContributionT{Token: contribution.Token, Bits: contribution.Bits}
	}

	symbols := make([]*telemetry.EnvelopeCognitionSymbolT, len(cognition.Symbols))

	for index, symbol := range cognition.Symbols {
		symbols[index] = &telemetry.EnvelopeCognitionSymbolT{
			Symbol:    symbol.Symbol,
			ClassName: symbol.Class,
			Score:     symbol.Score,
			Purity:    symbol.Purity,
		}
	}

	lexical := make([]*telemetry.EnvelopeCognitionLexicalT, len(cognition.Lexical))

	for index, entry := range cognition.Lexical {
		lexical[index] = &telemetry.EnvelopeCognitionLexicalT{
			Original:   entry.Original,
			Mapped:     entry.Mapped,
			Similarity: entry.Similarity,
		}
	}

	encoded := &telemetry.EnvelopeCognitionT{
		Source:                cognition.Source,
		Symbol:                cognition.Symbol,
		AtNs:                  timeNs(cognition.At),
		Sequence:              cognition.Sequence,
		RegimePrefix:          cognition.RegimePrefix,
		Winner:                cognition.Winner,
		WinnerClass:           cognition.WinnerClass,
		CandidateWinner:       cognition.CandidateWinner,
		StateHeld:             cognition.StateHeld,
		PredictionsHeld:       cognition.PredictionsHeld,
		SwitchConfidence:      cognition.SwitchConfidence,
		SwitchThreshold:       cognition.SwitchThreshold,
		Error:                 cognition.Error,
		Confidence:            cognition.Confidence,
		ClassConfidence:       cognition.ClassConfidence,
		Contrast:              cognition.Contrast,
		ContrastEvidence:      cognition.ContrastEvidence,
		Ambiguous:             cognition.Ambiguous,
		Cohort:                cognition.Cohort,
		LookaheadScore:        cognition.LookaheadScore,
		LookaheadPaths:        int64(cognition.LookaheadPaths),
		BeamWidth:             int64(cognition.BeamWidth),
		MaxHops:               int64(cognition.MaxHops),
		NodeCount:             int64(cognition.NodeCount),
		Predictions:           predictions,
		Branches:              branches,
		Beams:                 beams,
		Classes:               classes,
		RemFromNs:             timeNs(cognition.REMFrom),
		RemThroughNs:          timeNs(cognition.REMThrough),
		RemReplays:            int64(cognition.REMReplays),
		RemDecayFactor:        cognition.REMDecayFactor,
		RemInhibitionPct:      cognition.REMInhibitionPct,
		RemConsolidating:      cognition.REMConsolidating,
		InterpolatedSurprisal: cognition.InterpolatedSurprisal,
		Contributions:         contributions,
		Symbols:               symbols,
		Lexical:               lexical,
		Dreams:                cognition.Dreams,
	}

	if cognition.EntropyBits != nil {
		encoded.HasEntropyBits = true
		encoded.EntropyBits = *cognition.EntropyBits
	}

	if cognition.EntropyThreshold != nil {
		encoded.HasEntropyThreshold = true
		encoded.EntropyThreshold = *cognition.EntropyThreshold
	}

	return encoded
}

func encodeFrame(frame nmtypes.Frame) *telemetry.EnvelopeFrameT {
	mask := make([]uint64, len(frame.Mask))

	copy(mask, frame.Mask[:])

	data := make([]float64, len(frame.Data))
	copy(data, frame.Data[:])

	return &telemetry.EnvelopeFrameT{Mask: mask, Data: data}
}

/*
encodeResonanceDynamics reads the physical dynamics frame's slots by name onto
the wire. The frame itself crosses as an opaque mask/data pair, which a
consumer cannot pull a named quantity out of, so the named projection is what
makes the continuous dynamics readable at all.
*/
func encodeResonanceDynamics(dynamics nmtypes.Frame) *telemetry.EnvelopeResonanceDynamicsT {
	value := func(symbol nmtypes.Symbol) float64 {
		reading, _ := dynamics.Get(symbol)

		return reading
	}

	return &telemetry.EnvelopeResonanceDynamicsT{
		Ready:              value(learning.SymbolDynamicsReady),
		DeltaTime:          value(learning.SymbolDynamicsDeltaTime),
		Position:           value(learning.SymbolDynamicsPosition),
		Velocity:           value(learning.SymbolDynamicsVelocity),
		Acceleration:       value(learning.SymbolDynamicsAcceleration),
		Memory:             value(learning.SymbolDynamicsMemory),
		MemoryScale:        value(learning.SymbolDynamicsMemoryScale),
		StoredEnergy:       value(learning.SymbolDynamicsStoredEnergy),
		SuppliedPower:      value(learning.SymbolDynamicsSuppliedPower),
		Dissipation:        value(learning.SymbolDynamicsDissipation),
		PassivityResidue:   value(learning.SymbolDynamicsPassivityResidue),
		ContinuousVariance: value(learning.SymbolDynamicsContinuousVariance),
		JumpAmplitude:      value(learning.SymbolDynamicsJumpAmplitude),
		JumpVariance:       value(learning.SymbolDynamicsJumpVariance),
		SampleCount:        value(learning.SymbolDynamicsSampleCount),
		RotorScalar:        value(learning.SymbolDynamicsRotorScalar),
		RotorBivector:      value(learning.SymbolDynamicsRotorBivector),
		EquivarianceNorm:   value(learning.SymbolDynamicsEquivarianceNorm),
	}
}

func encodeResonanceArtifact(resonance *ResonanceArtifact) *telemetry.EnvelopeResonanceArtifactT {
	if resonance == nil {
		return nil
	}

	encoded := &telemetry.EnvelopeResonanceArtifactT{
		Symbol:               resonance.Symbol,
		AtNs:                 timeNs(resonance.At),
		Dynamics:             encodeFrame(resonance.Dynamics),
		ForwardCurve:         resonance.ForwardCurve,
		ForwardRetention:     resonance.ForwardRetention,
		SupportedHorizon:     int64(resonance.SupportedHorizon),
		Calibrated:           resonance.Calibrated,
		ResolvedSteps:        int64(resonance.ResolvedSteps),
		Readout:              resonance.Readout,
		Confidence:           resonance.Confidence,
		LastResolutionTarget: resonance.LastResolutionTarget,
		LastResolutionError:  resonance.LastResolutionError,
	}

	if manifold := resonance.Manifold; manifold != nil {
		layers, surprise, energy := manifold.WireSnapshot()
		encoded.Layers = make([]*telemetry.EnvelopeResonanceLayerT, 0, len(layers))

		for _, layer := range layers {
			encoded.Layers = append(encoded.Layers, &telemetry.EnvelopeResonanceLayerT{
				State:      layer.State,
				Prediction: layer.Prediction,
				ErrorNorm:  layer.ErrorNorm,
				Temporal:   layer.Temporal,
			})
		}

		encoded.Latent = manifold.LatentState()
		encoded.Energy = energy
		encoded.Surprise = surprise
		encoded.TaskSkill, encoded.TaskSkillReady = manifold.TaskSkill()
		encoded.TaskRelativePrecision, encoded.TaskRelativePrecisionReady = manifold.TaskPrecision()
		encoded.TaskScale, encoded.TaskScaleReady = manifold.TaskScale()
	}

	encoded.DynamicsNamed = encodeResonanceDynamics(resonance.Dynamics)

	if resonance.Forecast != nil {
		encoded.Forecast = &telemetry.EnvelopeReturnForecastT{
			Distribution: &telemetry.EnvelopeReturnForecastDistributionT{
				Value:            resonance.Forecast.Distribution.Value,
				Scale:            resonance.Forecast.Distribution.Scale,
				DegreesOfFreedom: resonance.Forecast.Distribution.DegreesOfFreedom,
				Ready:            resonance.Forecast.Distribution.Ready,
				Innovation:       resonance.Forecast.Distribution.Innovation,
				Reset:            resonance.Forecast.Distribution.Reset,
			},
			Horizon:          int64(resonance.Forecast.Horizon),
			CandidateCall:    resonance.Forecast.CandidateCall,
			Call:             resonance.Forecast.Call,
			StableCall:       resonance.Forecast.StableCall,
			Held:             resonance.Forecast.Held,
			SwitchConfidence: resonance.Forecast.SwitchConfidence,
			SwitchThreshold:  resonance.Forecast.SwitchThreshold,
		}
	}

	return encoded
}

func encodeManifoldState(manifold *ManifoldState) *telemetry.EnvelopeManifoldStateT {
	if manifold == nil {
		return nil
	}

	modes := make([]*telemetry.EnvelopeWaveModeT, len(manifold.Modes))

	for index, mode := range manifold.Modes {
		modes[index] = &telemetry.EnvelopeWaveModeT{
			Omega:     mode.Omega,
			Real:      mode.Real,
			Imaginary: mode.Imag,
			Linewidth: mode.Linewidth,
		}
	}

	return &telemetry.EnvelopeManifoldStateT{
		State: &telemetry.EnvelopeSensoriumStateT{
			N:          int64(manifold.State.N),
			Bytes:      manifold.State.Bytes,
			Seqs:       manifold.State.Seqs,
			TokenIds:   manifold.State.TokenIDs,
			ContentIds: manifold.State.ContentIDs,
			Phase:      manifold.State.Phase,
			Omega:      manifold.State.Omega,
			Energy:     manifold.State.Energy,
			Mass:       manifold.State.Mass,
			Heat:       manifold.State.Heat,
			Amp:        manifold.State.Amp,
			Pos:        manifold.State.Pos,
			Vel:        manifold.State.Vel,
			Clamped:    manifold.State.Clamped,
			Dark:       manifold.State.Dark,
		},
		Reading: &telemetry.EnvelopeSensoriumReadingT{
			Divergence:       manifold.Reading.Divergence,
			GuidanceSpeed:    manifold.Reading.GuidanceSpeed,
			CoherenceMag2:    manifold.Reading.CoherenceMag2,
			PressureGradNorm: manifold.Reading.PressureGradNorm,
			ViscosityProxy:   manifold.Reading.ViscosityProxy,
			KuramotoR:        manifold.Reading.KuramotoR,
		},
		GridX:         int64(manifold.GridX),
		GridY:         int64(manifold.GridY),
		GridZ:         int64(manifold.GridZ),
		GridSpacing:   manifold.GridSpacing,
		MomRho:        manifold.MomRho,
		FieldEnergy:   manifold.FieldEnergy,
		WaveReal:      manifold.WaveReal,
		WaveImag:      manifold.WaveImag,
		DensityScale:  manifold.DensityScale,
		MomentumScale: manifold.MomentumScale,
		EnergyScale:   manifold.EnergyScale,
		WaveScale:     manifold.WaveScale,
		Modes:         modes,
	}
}

func encodeTickerData(ticker kraken.TickerData) *telemetry.EnvelopeTickerDataT {
	encoded := &telemetry.EnvelopeTickerDataT{
		Symbol:      ticker.Symbol,
		BidQty:      ticker.BidQty,
		AskQty:      ticker.AskQty,
		Volume:      ticker.Volume,
		Vwap:        ticker.Vwap,
		ChangePct:   ticker.ChangePct,
		TimestampNs: timeNs(ticker.Timestamp),
	}

	if ticker.Bid != nil {
		encoded.HasBid = true
		encoded.Bid = ticker.Bid.Float64()
	}

	if ticker.Ask != nil {
		encoded.HasAsk = true
		encoded.Ask = ticker.Ask.Float64()
	}

	if ticker.Last != nil {
		encoded.HasLast = true
		encoded.Last = ticker.Last.Float64()
	}

	if ticker.Low != nil {
		encoded.HasLow = true
		encoded.Low = ticker.Low.Float64()
	}

	if ticker.High != nil {
		encoded.HasHigh = true
		encoded.High = ticker.High.Float64()
	}

	if ticker.Change != nil {
		encoded.HasChange = true
		encoded.Change = ticker.Change.Float64()
	}

	return encoded
}

func encodeTradeData(trade kraken.TradeData) *telemetry.EnvelopeTradeDataT {
	price := trade.Price
	return &telemetry.EnvelopeTradeDataT{
		Symbol:      trade.Symbol,
		Side:        trade.Side,
		Price:       price.Float64(),
		Qty:         trade.Qty,
		OrderType:   trade.OrderType,
		TradeId:     trade.TradeID,
		TimestampNs: timeNs(trade.Timestamp),
	}
}

func encodeFuturesTickerData(ticker kraken.FuturesTickerData) *telemetry.EnvelopeFuturesTickerDataT {
	encoded := &telemetry.EnvelopeFuturesTickerDataT{
		ProductId:    ticker.ProductID,
		Symbol:       ticker.Symbol,
		BidSize:      ticker.BidSize,
		AskSize:      ticker.AskSize,
		OpenInterest: ticker.OpenInterest,
		Volume:       ticker.Volume,
		TimestampNs:  timeNs(ticker.Timestamp),
	}

	if ticker.Bid != nil {
		encoded.HasBid = true
		encoded.Bid = ticker.Bid.Float64()
	}

	if ticker.Ask != nil {
		encoded.HasAsk = true
		encoded.Ask = ticker.Ask.Float64()
	}

	if ticker.Last != nil {
		encoded.HasLast = true
		encoded.Last = ticker.Last.Float64()
	}

	if ticker.MarkPrice != nil {
		encoded.HasMarkPrice = true
		encoded.MarkPrice = ticker.MarkPrice.Float64()
	}

	if ticker.IndexPrice != nil {
		encoded.HasIndexPrice = true
		encoded.IndexPrice = ticker.IndexPrice.Float64()
	}

	if ticker.FundingRate != nil {
		encoded.HasFundingRate = true
		encoded.FundingRate = ticker.FundingRate.Float64()
	}

	if ticker.FundingRatePrediction != nil {
		encoded.HasFundingRatePrediction = true
		encoded.FundingRatePrediction = ticker.FundingRatePrediction.Float64()
	}

	return encoded
}

func encodeFuturesTradeData(trade kraken.FuturesTradeData) *telemetry.EnvelopeFuturesTradeDataT {
	price := trade.Price
	return &telemetry.EnvelopeFuturesTradeDataT{
		ProductId:   trade.ProductID,
		Symbol:      trade.Symbol,
		Price:       price.Float64(),
		Qty:         trade.Qty,
		Side:        trade.Side,
		Type:        trade.Type,
		Uid:         trade.UID,
		TimestampNs: timeNs(trade.Timestamp),
	}
}

func encodeLevel3Orders(orders []kraken.Level3Order) []*telemetry.EnvelopeLevel3OrderT {
	if orders == nil {
		return nil
	}

	encoded := make([]*telemetry.EnvelopeLevel3OrderT, len(orders))

	for index, order := range orders {
		out := &telemetry.EnvelopeLevel3OrderT{
			Event:       order.Event,
			OrderId:     order.OrderID,
			TimestampNs: timeNs(order.Timestamp),
		}

		if order.LimitPrice != nil {
			out.HasLimitPrice = true
			out.LimitPrice = order.LimitPrice.Float64()
		}

		if order.OrderQty != nil {
			out.HasOrderQty = true
			out.OrderQty = order.OrderQty.Float64()
		}

		encoded[index] = out
	}

	return encoded
}

func encodeLevel3Data(level3 kraken.Level3Data) *telemetry.EnvelopeLevel3DataT {
	return &telemetry.EnvelopeLevel3DataT{
		Symbol:      level3.Symbol,
		Type:        level3.Type,
		TimestampNs: timeNs(level3.Timestamp),
		Checksum:    level3.Checksum,
		Bids:        encodeLevel3Orders(level3.Bids),
		Asks:        encodeLevel3Orders(level3.Asks),
	}
}

func encodeBoundaries(boundaries []BoundaryStamp) []*telemetry.EnvelopeBoundaryStampT {
	if boundaries == nil {
		return nil
	}

	encoded := make([]*telemetry.EnvelopeBoundaryStampT, len(boundaries))

	for i, stamp := range boundaries {
		encoded[i] = &telemetry.EnvelopeBoundaryStampT{
			Label:     stamp.Label,
			AtNs:      stamp.AtNs,
			SeqCount:  stamp.SeqCount,
			AvgGapNs:  stamp.AvgGapNs,
			LastGapNs: stamp.LastGapNs,
			Backlog:   stamp.Backlog,
			Group:     stamp.Group,
			Stage:     stamp.Stage,
		}
	}

	return encoded
}

/*
EquityReading is one account valuation: the cash balance, the unrealized
profit/loss on open positions, and the total equity of the two together.

The values are carried as their decimal strings rather than float64 because
that is what the broker reports and what the wire type declares; rounding an
account balance through a binary float to save three string fields is not a
trade worth making.
*/
type EquityReading struct {
	Cash       string
	Unrealized string
	Equity     string
}

/*
NewEquityReading projects a broker trade balance into an EquityReading.

A valuation is only meaningful once the broker has reported a total equity, so
a balance without one yields no reading rather than a reading of zeros — the
dashboard's "—" is the honest rendering of "not yet known", and a fabricated
0.00 would be indistinguishable from a genuinely empty account.

Cash is the trade balance (the account's own funds) and Unrealized is the
open-position profit/loss, matching how Desk.PublishEquity documents the three.
*/
func NewEquityReading(balance kraken.TradeBalanceResult) *EquityReading {
	if balance.Equity == nil {
		return nil
	}

	reading := &EquityReading{Equity: balance.Equity.String()}

	if balance.TradeBalance != nil {
		reading.Cash = balance.TradeBalance.String()
	}

	if balance.UnrealizedPnL != nil {
		reading.Unrealized = balance.UnrealizedPnL.String()
	}

	return reading
}

/*
encodeEquity projects the account valuation onto the wire. A nil reading stays
absent, so an envelope produced before the first broker valuation carries no
equity rather than a zeroed one.
*/
func encodeEquity(reading *EquityReading) *telemetry.EquityFrameT {
	if reading == nil {
		return nil
	}

	return &telemetry.EquityFrameT{
		Cash:       reading.Cash,
		Unrealized: reading.Unrealized,
		Equity:     reading.Equity,
	}
}

/*
encodePositions wraps the desk's open-lot rows in a PositionsFrame. A nil or
empty slice stays absent, so an envelope produced before any position exists
carries no frame rather than an empty one.
*/
func encodePositions(positions []*telemetry.PositionT) *telemetry.PositionsFrameT {
	if len(positions) == 0 {
		return nil
	}

	return &telemetry.PositionsFrameT{Rows: positions}
}

/*
Encode converts the Envelope's exported state into its FlatBuffers mirror,
verbatim: every populated field crosses as itself, with only the type
coercions FlatBuffers requires (time.Time -> UnixNano, *decimal.Decimal ->
float64 via .Float64(), enums -> their string form). A nil/zero field on the
Envelope stays absent/zero on the wire type.
*/
func (envelope *Envelope) Encode() *telemetry.EnvelopeStateT {
	if envelope == nil {
		return nil
	}

	state := envelope.encodeBase(envelopeMeasurementIdentity)
	state.Resonance = encodeResonanceArtifact(envelope.Resonance)
	state.Manifold = encodeManifoldState(envelope.Manifold)
	state.Boundaries = encodeBoundaries(envelope.Boundaries)

	return state
}

/*
measurementForFocus gates one signal measurement for the dashboard publish
path: only the focused symbol's measurement crosses the websocket. A
non-focused symbol is dropped (nil) here — and only here, at the signal
measurement fields — so the ticker/trade data, equity, positions, resonance,
categories and the trading thesis path are never touched. The symbol identity
is the measurement's own Label, which the projector set from the observation
symbol, so this never depends on the envelope Key.
*/
func measurementForFocus(measurement *data.Measurement[float64]) *data.Measurement[float64] {
	if measurement == nil || measurement.Label == Focus() {
		return measurement
	}

	return nil
}

/*
envelopeMeasurementIdentity is the persistence-path gate: Hindsight capture and
historical reads keep every signal measurement regardless of the dashboard's
live focus, so the audit record never loses a non-focused symbol's readings.
*/
func envelopeMeasurementIdentity(measurement *data.Measurement[float64]) *data.Measurement[float64] {
	return measurement
}

/*
encodeBase builds the shared control/state projection of the Envelope: every
ordinary field crosses over. Resonance, Manifold, and Boundaries are withheld
here because all three ride their own WebRTC channels (the dashboard's
predictive-coding, manifold, and topology surfaces consume them there); the
websocket carries only lean, latency-relevant state. Only the heavy Manifold
encoder is withheld from the base projection; Resonance and Boundaries are
re-added by the full Encode() path for durable Hindsight persistence. Both
Encode (full) and EncodeWebsocket (observer) share this projection so their
fields cannot drift. measurementGate lets a caller decide which signal
measurements cross (the websocket drops non-focus symbols); the persistence
path passes the identity gate so Hindsight capture keeps every measurement.
*/
func (envelope *Envelope) encodeBase(
	measurementGate func(*data.Measurement[float64]) *data.Measurement[float64],
) *telemetry.EnvelopeStateT {
	return &telemetry.EnvelopeStateT{
		Key:               envelope.Key,
		TypeId:            byte(envelope.TypeID),
		Tick:              envelope.Tick,
		CaptureRun:        string(envelope.CaptureID.Run),
		CaptureSeq:        uint64(envelope.CaptureID.Sequence),
		CaptureStream:     string(envelope.CaptureID.Stream),
		CaptureEpoch:      uint64(envelope.CaptureID.StreamEpoch),
		CaptureStreamSeq:  envelope.CaptureID.StreamSequence,
		CaptureOrdinal:    envelope.CaptureOrdinal,
		TickerData:        encodeTickerData(envelope.TickerData),
		TradeData:         encodeTradeData(envelope.TradeData),
		Level3Data:        encodeLevel3Data(envelope.Level3Data),
		FuturesTickerData: encodeFuturesTickerData(envelope.FuturesTickerData),
		FuturesTradeData:  encodeFuturesTradeData(envelope.FuturesTradeData),
		Correlation:       encodeMeasurement(measurementGate(envelope.Correlation)),
		LeadLag:           encodeMeasurement(measurementGate(envelope.LeadLag)),
		Liquidity:         encodeMeasurement(measurementGate(envelope.Liquidity)),
		Sentiment:         encodeMeasurement(measurementGate(envelope.Sentiment)),
		Cvd:               encodeMeasurement(measurementGate(envelope.CVD)),
		DepthFlow:         encodeMeasurement(measurementGate(envelope.DepthFlow)),
		Morphology:        encodeMeasurement(measurementGate(envelope.Morphology)),
		Hawkes:            encodeMeasurement(measurementGate(envelope.Hawkes)),
		PumpDump:          encodeMeasurement(measurementGate(envelope.PumpDump)),
		Toxicity:          encodeMeasurement(measurementGate(envelope.Toxicity)),
		Derivatives:       encodeMeasurement(measurementGate(envelope.Derivatives)),
		Categories:        encodeCategories(envelope.Categories),
		Opportunities:     encodeOpportunities(envelope.Opportunities),
		GraphUpdate:       encodeGraphUpdate(envelope.GraphUpdate),
		Cognition:         encodeCognition(envelope.Cognition),
		Strategy:          encodeStrategyRound(envelope.StrategyRound),
		Perspectives:      encodePerspectives(envelope.Perspectives),
		Equity:            encodeEquity(envelope.Equity),
		Positions:         encodePositions(envelope.Positions),
	}
}

/*
encodePerspectives projects the advisory layer's descriptive context into the
wire. Each Perspective becomes one PerspectiveFrame; each reading's interned
Metric symbol resolves to its name for serialization (diagnostics/UI only, never
a hot-path comparison).
*/
func encodePerspectives(perspectives []*Perspective) []*telemetry.PerspectiveFrameT {
	if len(perspectives) == 0 {
		return nil
	}

	frames := make([]*telemetry.PerspectiveFrameT, 0, len(perspectives))

	for _, perspective := range perspectives {
		if perspective == nil {
			continue
		}

		readings := make([]*telemetry.PerspectiveReadingT, 0, perspective.Count)

		for index := 0; index < perspective.Count; index++ {
			reading := perspective.Readings[index]
			name, _ := nmtypes.SymbolName(reading.Metric)

			readings = append(readings, &telemetry.PerspectiveReadingT{
				Metric:     name,
				Value:      reading.Value,
				Defined:    reading.Defined,
				ObservedAt: timeNs(reading.ObservedAt),
				From:       timeNs(reading.From),
				Maturity:   reading.Maturity,
				Snr:        reading.SNR,
				SnrDefined: reading.SNRDefined,
			})
		}

		frames = append(frames, &telemetry.PerspectiveFrameT{
			Symbol:   perspective.Symbol,
			Peer:     perspective.Peer,
			Kind:     uint8(perspective.Kind),
			At:       timeNs(perspective.At),
			Sequence: int64(perspective.Sequence),
			Readings: readings,
		})
	}

	return frames
}

/*
encodeStrategyRound projects the planner's decision round into the StrategyFrame
the dashboard's strategyStore renders. It is the strategy layer's one wire
output: the same decisions the planner executed, carried on the envelope that
bore the logic inputs producing them.
*/
func encodeStrategyRound(round *StrategyRound) *telemetry.StrategyFrameT {
	if round == nil {
		return nil
	}

	decisions := make([]*telemetry.DecisionT, 0, len(round.Decisions))

	for _, decision := range round.Decisions {
		if decision == nil {
			continue
		}

		decisions = append(decisions, encodeDecision(decision))
	}

	return &telemetry.StrategyFrameT{
		Evaluated: round.Evaluated,
		Outcome:   round.Outcome,
		Decisions: decisions,
	}
}

func encodeDecision(decision *Decision) *telemetry.DecisionT {
	if decision == nil {
		return nil
	}

	names := make([]string, 0, len(decision.Alternatives))

	for name := range decision.Alternatives {
		names = append(names, name)
	}

	sort.Strings(names)
	alternatives := make([]*telemetry.NamedNumberT, 0, len(names))

	for _, name := range names {
		alternatives = append(alternatives, &telemetry.NamedNumberT{
			Name:  name,
			Value: decision.Alternatives[name],
		})
	}

	return &telemetry.DecisionT{
		Id:               decision.ID,
		Action:           string(decision.Action),
		Symbol:           decision.Symbol,
		At:               timeNs(decision.At),
		Direction:        decision.Direction,
		Alternatives:     alternatives,
		AllocationClass:  decision.AllocationClass,
		Opportunity:      decision.Opportunity,
		OpportunityType:  string(decision.OpportunityType),
		OpportunityPhase: string(decision.OpportunityPhase),
		PredictiveReady:  decision.PredictiveReady,
		PredictiveStatus: decision.PredictiveStatus,
		TaskSkill:        decision.TaskSkill,
		TaskSkillReady:   decision.TaskSkillReady,
		ProposedNotional: decimalString(decision.ProposedNotional),
		ProposedQuantity: decimalString(decision.ProposedQuantity),
		ReferencePrice:   decimalString(decision.ReferencePrice),
		ForecastSource:   decision.ForecastSource,
		ForecastModel:    decision.ForecastModel,
		ForecastHorizon:  int64(decision.ForecastHorizon),
		CalibrationCount: decision.CalibrationCount,
		Confidence:       decision.Confidence,
		AvailableCapital: decimalString(decision.AvailableCapital),
		OpenPositions:    int64(decision.OpenPositions),
		Cause:            decision.Cause,
		Reason:           decision.Reason,
		ReservationId:    decision.ReservationID,
		SellableQty:      decimalString(decision.SellableQty),
		EntryAt:          timePointerNano(decision.EntryAt),
		ExitAt:           timePointerNano(decision.ExitAt),
		EntryPrice:       decimalString(decision.EntryPrice),
		EntryFee:         decimalString(decision.EntryFee),
		ExitPrice:        decimalString(decision.ExitPrice),
		ExitFee:          decimalString(decision.ExitFee),
		Pnl:              decimalString(decision.PnL),
		ReturnPct:        floatPointer(decision.ReturnPct),
		Mark:             decimalString(decision.Mark),
		EntryCost:        entryCostWire(decision.EntryCost),
		Stoploss:         StoplossWire(decision.Stoploss),
		Risk:             riskWire(&decision.Risk),
	}
}

/*
EncodeBytes packs Encode's result into a standalone FlatBuffers buffer rooted
at EnvelopeState — no Frame/Envelope transport wrapper — for callers that send
or store the full mirror directly, such as the Hindsight persistence writer.
*/
func (envelope *Envelope) EncodeBytes() []byte {
	builder := flatbuffers.NewBuilder(0)
	offset := envelope.Encode().Pack(builder)
	telemetry.FinishEnvelopeStateBuffer(builder, offset)

	return builder.FinishedBytes()
}

/*
EncodeWebsocket packs the envelope's lean websocket mirror directly, never
calling the heavy Manifold encoder. Manifold, Resonance, and Boundaries all
ride their own WebRTC channels, so the dashboard socket carries only the
latency-relevant state and none of the volumetric fields. It shares the base
projection with Encode so their ordinary fields cannot drift.
*/
func (envelope *Envelope) EncodeWebsocket() []byte {
	if envelope == nil {
		return nil
	}

	builder := flatbuffers.NewBuilder(0)
	offset := envelope.encodeBase(measurementForFocus).Pack(builder)
	telemetry.FinishEnvelopeStateBuffer(builder, offset)

	return builder.FinishedBytes()
}

/*
EncodeResonanceArtifactWire projects only the resonance artifact into its wire
mirror, for the WebRTC resonance channel. It reuses the same encoder Encode
uses so the WebRTC frame and the (now omitted) websocket field can never drift.
*/
func (envelope *Envelope) EncodeResonanceArtifactWire() *telemetry.EnvelopeResonanceArtifactT {
	return encodeResonanceArtifact(envelope.Resonance)
}

/*
EncodeManifoldWire projects only the resident manifold advance into its wire
mirror, for the WebRTC manifold channel.
*/
func (envelope *Envelope) EncodeManifoldWire() *telemetry.EnvelopeManifoldStateT {
	return encodeManifoldState(envelope.Manifold)
}

/*
EncodeBoundariesWire projects only the ordered boundary trace into its wire
mirror, for the WebRTC diagnostics channel.
*/
func (envelope *Envelope) EncodeBoundariesWire() []*telemetry.EnvelopeBoundaryStampT {
	return encodeBoundaries(envelope.Boundaries)
}
