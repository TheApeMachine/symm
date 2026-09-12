package types

import (
	"hash/fnv"
	"time"

	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/transport"
	"golang.design/x/lockfree/lf"
)

/*
cloneMeasurement deep-copies one measurement through the data.Cloner primitive.
A nil measurement clones to nil.
*/
func cloneMeasurement(measurement *data.Measurement[float64]) *data.Measurement[float64] {
	var cloned *data.Measurement[float64]

	for out := range data.NewCloner[float64]().Next(transport.NewValues(measurement).Next(nil)) {
		cloned = *(**data.Measurement[float64])(out)
	}

	return cloned
}

/*
MarketCut is the unified, concurrent, streaming state cut of the market for one symbol.
It anchors the contemporaneous state across disjoint incoming entities (ticker, trade, level3)
and asynchronous physics advances.
*/
type MarketCut struct {
	Symbol     string
	At         time.Time
	Ticker     kraken.TickerData
	Trade      kraken.TradeData
	Level3     kraken.Level3Data
	Manifold   *ManifoldState
	Resonance  *ResonanceArtifact
	Categories []Category
	Cognition  *Cognition

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
}

/*
MarketState owns the lock-free collection of streaming MarketCuts across all symbols.
*/
type MarketState struct {
	cuts *lf.HashMap[string, *MarketCut]
}

func NewMarketState() *MarketState {
	hash := func(s string) uint64 {
		h := fnv.New64a()
		_, _ = h.Write([]byte(s))
		return h.Sum64()
	}

	less := func(a, b string) bool {
		return a < b
	}

	return &MarketState{
		cuts: lf.NewHashMap[string, *MarketCut](64, hash, less),
	}
}

func (state *MarketState) Cut(symbol string) *MarketCut {
	if state == nil || state.cuts == nil || symbol == "" {
		return nil
	}

	cut, found := state.cuts.Get(symbol)
	if !found {
		return nil
	}

	return cut
}

/*
Update folds the latest observation and measurements from an envelope into the symbol's market cut.
*/
func (state *MarketState) Update(envelope *Envelope) {
	if state == nil || state.cuts == nil || envelope == nil {
		return
	}

	symbol := envelope.Symbol()
	if symbol == "" {
		return
	}

	cut, found := state.cuts.Get(symbol)
	if !found || cut == nil {
		cut = &MarketCut{
			Symbol: symbol,
		}
	}

	cut.At = time.Now()

	switch envelope.TypeID {
	case EnvelopeTicker:
		cut.Ticker = envelope.TickerData
	case EnvelopeTrade:
		cut.Trade = envelope.TradeData
	case EnvelopeLevel3:
		cut.Level3 = envelope.Level3Data
	}

	if envelope.Correlation != nil {
		cut.Correlation = cloneMeasurement(envelope.Correlation)
	}
	if envelope.LeadLag != nil {
		cut.LeadLag = cloneMeasurement(envelope.LeadLag)
	}
	if envelope.Liquidity != nil {
		cut.Liquidity = cloneMeasurement(envelope.Liquidity)
	}
	if envelope.Sentiment != nil {
		cut.Sentiment = cloneMeasurement(envelope.Sentiment)
	}
	if envelope.CVD != nil {
		cut.CVD = cloneMeasurement(envelope.CVD)
	}
	if envelope.DepthFlow != nil {
		cut.DepthFlow = cloneMeasurement(envelope.DepthFlow)
	}
	if envelope.Morphology != nil {
		cut.Morphology = cloneMeasurement(envelope.Morphology)
	}
	if envelope.Hawkes != nil {
		cut.Hawkes = cloneMeasurement(envelope.Hawkes)
	}
	if envelope.PumpDump != nil {
		cut.PumpDump = cloneMeasurement(envelope.PumpDump)
	}
	if envelope.Toxicity != nil {
		cut.Toxicity = cloneMeasurement(envelope.Toxicity)
	}
	if envelope.Derivatives != nil {
		cut.Derivatives = cloneMeasurement(envelope.Derivatives)
	}
	if envelope.Resonance != nil {
		cut.Resonance = envelope.Resonance
	}
	if envelope.Manifold != nil {
		cut.Manifold = envelope.Manifold
	}
	if len(envelope.Categories) > 0 {
		cut.Categories = envelope.Categories
	}
	if envelope.Cognition != nil {
		cut.Cognition = envelope.Cognition
	}

	state.cuts.Set(symbol, cut)
}

/*
UpdateManifold updates the symbol's latest Manifold physics state directly from the solver advance.
*/
func (state *MarketState) UpdateManifold(symbol string, manifold *ManifoldState) {
	if state == nil || state.cuts == nil || symbol == "" || manifold == nil {
		return
	}

	cut, found := state.cuts.Get(symbol)
	if !found || cut == nil {
		cut = &MarketCut{
			Symbol: symbol,
		}
	}

	cut.Manifold = manifold
	state.cuts.Set(symbol, cut)
}

/*
Hydrate populates missing contemporaneous fields onto the envelope from the latest cut.
This ensures downstream learning and evaluation observe complete market state.
*/
func (state *MarketState) Hydrate(envelope *Envelope) {
	if state == nil || state.cuts == nil || envelope == nil {
		return
	}

	symbol := envelope.Symbol()
	if symbol == "" {
		return
	}

	cut, found := state.cuts.Get(symbol)
	if !found || cut == nil {
		return
	}

	if envelope.Correlation == nil {
		envelope.Correlation = cloneMeasurement(cut.Correlation)
	}
	if envelope.LeadLag == nil {
		envelope.LeadLag = cloneMeasurement(cut.LeadLag)
	}
	if envelope.Liquidity == nil {
		envelope.Liquidity = cloneMeasurement(cut.Liquidity)
	}
	if envelope.Sentiment == nil {
		envelope.Sentiment = cloneMeasurement(cut.Sentiment)
	}
	if envelope.CVD == nil {
		envelope.CVD = cloneMeasurement(cut.CVD)
	}
	if envelope.DepthFlow == nil {
		envelope.DepthFlow = cloneMeasurement(cut.DepthFlow)
	}
	if envelope.Morphology == nil {
		envelope.Morphology = cloneMeasurement(cut.Morphology)
	}
	if envelope.Hawkes == nil {
		envelope.Hawkes = cloneMeasurement(cut.Hawkes)
	}
	if envelope.PumpDump == nil {
		envelope.PumpDump = cloneMeasurement(cut.PumpDump)
	}
	if envelope.Toxicity == nil {
		envelope.Toxicity = cloneMeasurement(cut.Toxicity)
	}
	if envelope.Derivatives == nil {
		envelope.Derivatives = cloneMeasurement(cut.Derivatives)
	}
	if envelope.Resonance == nil {
		envelope.Resonance = cut.Resonance
	}
	if envelope.Manifold == nil {
		envelope.Manifold = cut.Manifold
	}
	if len(envelope.Categories) == 0 {
		envelope.Categories = cut.Categories
	}
	if envelope.Cognition == nil {
		envelope.Cognition = cut.Cognition
	}
}

/*
Step implements nomagique/runtime.Node[*Envelope].
It updates the streaming cut with incoming observations and hydrates
missing contemporaneous fields onto the envelope so downstream stages
observe a complete market cut.
*/
func (state *MarketState) Step(envelope *Envelope) *Envelope {
	if state == nil || envelope == nil {
		return envelope
	}

	state.Update(envelope)
	state.Hydrate(envelope)

	return envelope
}

func (state *MarketState) Name() string { return "market_state" }
func (state *MarketState) Error() error { return nil }
