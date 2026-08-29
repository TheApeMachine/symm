package types

import (
	"time"

	flatbuffers "github.com/google/flatbuffers/go"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/nomagique/data"
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
BoundaryStamp is one Observe group's "the envelope passed here" witness: the
group's fixed slot index (see runtime.BoundarySlot*) and when it observed the
envelope. A diagnostics Observe node derives each Compute stage's elapsed time
by subtracting consecutive boundary stamps — it never times a Compute Node
itself, since timing computation is a Compute concern (README §12/§13) and an
Observe node may only witness already-committed state.
*/
type BoundaryStamp struct {
	AtNs int64
}

/*
BoundarySlots bounds Envelope.Boundaries. Every Observe group in a Workload's
declared graph is assigned one fixed, exclusive slot at construction (see the
runtime.BoundarySlot constants) — disjoint real memory, since an Observe
group's own Nodes could otherwise race on a shared array cell the same way
concurrent Compute Nodes must never share a slice header (README §8).
*/
const BoundarySlots = 8

type TypeID uint8

const (
	EnvelopeUnknown TypeID = iota
	EnvelopeTicker
	EnvelopeTrade
	EnvelopeLevel3
	EnvelopeExecution
)

type Envelope struct {
	Key    string
	TypeID TypeID

	TickerData kraken.TickerData
	TradeData  kraken.TradeData
	Level3Data kraken.Level3Data

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

	// CausalOutput is the causal stage's Pearl-ladder evaluation, resolved
	// from Resonance.
	CausalOutput *CausalOutput

	// Boundaries is every Observe group's "passed here at T" witness, one
	// fixed slot per boundary (see BoundarySlots/runtime.BoundarySlot*). A
	// diagnostics Observe node reads consecutive slots to report elapsed
	// Compute-stage time; the Compute stage itself never writes here.
	Boundaries [BoundarySlots]BoundaryStamp
}

func NewEnvelope(typeID TypeID) *Envelope {
	return &Envelope{
		TypeID: typeID,
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

func encodeCausalRowValue(value any) *telemetry.EnvelopeAnyValueT {
	switch typed := value.(type) {
	case string:
		return &telemetry.EnvelopeAnyValueT{Type: telemetry.EnvelopeAnyValueNamedString, Value: &telemetry.NamedStringT{Value: typed}}
	case float64:
		return &telemetry.EnvelopeAnyValueT{Type: telemetry.EnvelopeAnyValueNamedNumber, Value: &telemetry.NamedNumberT{Value: typed}}
	case int:
		return &telemetry.EnvelopeAnyValueT{Type: telemetry.EnvelopeAnyValueEnvelopeNamedInt, Value: &telemetry.EnvelopeNamedIntT{Value: int64(typed)}}
	case int64:
		return &telemetry.EnvelopeAnyValueT{Type: telemetry.EnvelopeAnyValueEnvelopeNamedInt, Value: &telemetry.EnvelopeNamedIntT{Value: typed}}
	case time.Time:
		return &telemetry.EnvelopeAnyValueT{Type: telemetry.EnvelopeAnyValueEnvelopeNamedTimeNs, Value: &telemetry.EnvelopeNamedTimeNsT{ValueNs: timeNs(typed)}}
	case [][]float64:
		rows := make([]*telemetry.EnvelopeFloatRowT, len(typed))

		for index, row := range typed {
			rows[index] = &telemetry.EnvelopeFloatRowT{Values: row}
		}

		return &telemetry.EnvelopeAnyValueT{Type: telemetry.EnvelopeAnyValueEnvelopeNamedFloatMatrix, Value: &telemetry.EnvelopeNamedFloatMatrixT{Rows: rows}}
	default:
		return nil
	}
}

func encodeCausalOutput(causal *CausalOutput) *telemetry.EnvelopeCausalOutputT {
	if causal == nil {
		return nil
	}

	rows := make([]*telemetry.EnvelopeAnyEntryT, 0, len(causal.Rows))

	for key, value := range causal.Rows {
		encodedValue := encodeCausalRowValue(value)

		if encodedValue == nil {
			continue
		}

		rows = append(rows, &telemetry.EnvelopeAnyEntryT{Key: key, Value: encodedValue})
	}

	return &telemetry.EnvelopeCausalOutputT{Symbol: causal.Symbol, Rows: rows}
}

func encodeFrame(frame nmtypes.Frame) *telemetry.EnvelopeFrameT {
	mask := make([]uint64, len(frame.Mask))

	copy(mask, frame.Mask[:])

	data := make([]float64, len(frame.Data))
	copy(data, frame.Data[:])

	return &telemetry.EnvelopeFrameT{Mask: mask, Data: data}
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

func encodeBoundaries(boundaries [BoundarySlots]BoundaryStamp) []*telemetry.EnvelopeBoundaryStampT {
	encoded := make([]*telemetry.EnvelopeBoundaryStampT, 0, BoundarySlots)

	for slot, stamp := range boundaries {
		if stamp.AtNs == 0 {
			continue
		}

		encoded = append(encoded, &telemetry.EnvelopeBoundaryStampT{Slot: int32(slot), AtNs: stamp.AtNs})
	}

	return encoded
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

	return &telemetry.EnvelopeStateT{
		Key:           envelope.Key,
		TypeId:        byte(envelope.TypeID),
		TickerData:    encodeTickerData(envelope.TickerData),
		TradeData:     encodeTradeData(envelope.TradeData),
		Level3Data:    encodeLevel3Data(envelope.Level3Data),
		Correlation:   encodeMeasurement(envelope.Correlation),
		LeadLag:       encodeMeasurement(envelope.LeadLag),
		Liquidity:     encodeMeasurement(envelope.Liquidity),
		Sentiment:     encodeMeasurement(envelope.Sentiment),
		Cvd:           encodeMeasurement(envelope.CVD),
		DepthFlow:     encodeMeasurement(envelope.DepthFlow),
		Morphology:    encodeMeasurement(envelope.Morphology),
		Hawkes:        encodeMeasurement(envelope.Hawkes),
		PumpDump:      encodeMeasurement(envelope.PumpDump),
		Toxicity:      encodeMeasurement(envelope.Toxicity),
		Categories:    encodeCategories(envelope.Categories),
		Opportunities: encodeOpportunities(envelope.Opportunities),
		GraphUpdate:   encodeGraphUpdate(envelope.GraphUpdate),
		Resonance:     encodeResonanceArtifact(envelope.Resonance),
		Manifold:      encodeManifoldState(envelope.Manifold),
		Cognition:     encodeCognition(envelope.Cognition),
		CausalOutput:  encodeCausalOutput(envelope.CausalOutput),
		Boundaries:    encodeBoundaries(envelope.Boundaries),
	}
}

/*
EncodeBytes packs Encode's result into a standalone FlatBuffers buffer rooted
at EnvelopeState — no Frame/Envelope transport wrapper — for callers that send
or store the mirror directly, such as the UI websocket.
*/
func (envelope *Envelope) EncodeBytes() []byte {
	builder := flatbuffers.NewBuilder(0)
	offset := envelope.Encode().Pack(builder)
	telemetry.FinishEnvelopeStateBuffer(builder, offset)

	return builder.FinishedBytes()
}
