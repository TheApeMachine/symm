package types

import (
	"iter"
	"sort"
	"time"

	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/nomagique/transport"
	"github.com/theapemachine/symm/nomagique/types"
	wire "github.com/theapemachine/symm/telemetry/generated/telemetry"
)

/*
Symbol is an alternative grouping of measurements, and is used in logic that
legitimately needs to fan out measurements by symbol, rather than by source.
It should be kept extremely simple, and lean, and is not an invitation to
start adding additional complexity beyond what is truly earned.
*/
type Symbol struct {
	ui                   *transport.MapReduce[*UIFrame]              `json:"-"`
	notify               func(SourceType, *Symbol)                   `json:"-"`
	ID                   SymbolID                                    `json:"id,omitempty"`
	Symbol               string                                      `json:"symbol,omitempty"`
	Status               Status                                      `json:"status,omitempty"`
	Tick                 int64                                       `json:"tick,omitempty"`
	Measurements         *transport.MapReduce[*types.Measurement]    `json:"-"`
	TickerConsumers      []*transport.Consumer[kraken.TickerData]    `json:"-"`
	TradeConsumers       []*transport.Consumer[kraken.TradeData]     `json:"-"`
	Level3Consumers      []*transport.Consumer[kraken.Level3Data]    `json:"-"`
	ExecutionConsumers   []*transport.Consumer[kraken.ExecutionData] `json:"-"`
	MeasurementConsumers []*transport.Consumer[*types.Measurement]   `json:"-"`
	DecisionConsumers    []*transport.Consumer[Decision]             `json:"-"`
	PositionConsumers    []*transport.Consumer[any]                  `json:"-"`
	GraphConsumers       []*transport.Consumer[*Graph]               `json:"-"`
	CategoryConsumers    []*transport.Consumer[[]Category]           `json:"-"`
	PhaseConsumers       []*transport.Consumer[PhaseReading]         `json:"-"`
	CognitionConsumers   []*transport.Consumer[Cognition]            `json:"-"`
	ResonanceConsumers   []*transport.Consumer[any]                  `json:"-"`
	CausalConsumers      []*transport.Consumer[map[string]any]       `json:"-"`
	tickers              *transport.MapReduce[kraken.TickerData]     `json:"-"`
	trades               *transport.MapReduce[kraken.TradeData]      `json:"-"`
	level3               *transport.MapReduce[kraken.Level3Data]     `json:"-"`
	executions           *transport.MapReduce[kraken.ExecutionData]  `json:"-"`
	Decisions            *transport.MapReduce[Decision]              `json:"decisions,omitempty"`
	Positions            *transport.MapReduce[any]                   `json:"positions,omitempty"`
	Graphs               *transport.MapReduce[*Graph]                `json:"graphs,omitempty"`
	Categories           *transport.MapReduce[[]Category]            `json:"categories,omitempty"`
	Phase                *transport.MapReduce[PhaseReading]          `json:"-"`
	Cognition            *transport.MapReduce[Cognition]             `json:"-"`
	Resonance            *transport.MapReduce[any]                   `json:"-"`
	Causal               *transport.MapReduce[map[string]any]        `json:"-"`
}

/*
NewSymbol creates empty measurement state for one market symbol.
*/
func NewSymbol(name string, uiChannels ...*transport.MapReduce[*UIFrame]) *Symbol {
	var ui *transport.MapReduce[*UIFrame]

	if len(uiChannels) > 0 {
		ui = uiChannels[0]
	}

	symbol := &Symbol{Symbol: name, Status: READY, ui: ui}
	symbol.TickerConsumers = []*transport.Consumer[kraken.TickerData]{
		newSymbolConsumer[kraken.TickerData](symbol, SourceCorrelation),
		newSymbolConsumer[kraken.TickerData](symbol, SourceLeadLag),
		newSymbolConsumer[kraken.TickerData](symbol, SourceLiquidity),
		newSymbolConsumer[kraken.TickerData](symbol, SourcePumpDump),
		newSymbolConsumer[kraken.TickerData](symbol, SourceSentiment),
		newSymbolConsumer[kraken.TickerData](symbol, SourceResonance),
		newSymbolConsumer[kraken.TickerData](symbol, SourceDesk),
	}
	symbol.TradeConsumers = []*transport.Consumer[kraken.TradeData]{
		newSymbolConsumer[kraken.TradeData](symbol, SourceCVD),
		newSymbolConsumer[kraken.TradeData](symbol, SourceExhaustion),
		newSymbolConsumer[kraken.TradeData](symbol, SourceHawkes),
		newSymbolConsumer[kraken.TradeData](symbol, SourcePumpDump),
	}
	symbol.Level3Consumers = []*transport.Consumer[kraken.Level3Data]{
		newSymbolConsumer[kraken.Level3Data](symbol, SourceDepthFlow).Coalesce(),
		newSymbolConsumer[kraken.Level3Data](symbol, SourceToxicity),
		newSymbolConsumer[kraken.Level3Data](symbol, SourcePumpDump).Coalesce(),
	}
	symbol.ExecutionConsumers = []*transport.Consumer[kraken.ExecutionData]{
		newSymbolConsumer[kraken.ExecutionData](symbol, SourceDesk),
	}
	symbol.MeasurementConsumers = []*transport.Consumer[*types.Measurement]{
		newSymbolConsumer[*types.Measurement](symbol, SourceCategory),
		newSymbolConsumer[*types.Measurement](symbol, SourceManifold),
		newSymbolConsumer[*types.Measurement](symbol, SourceGraph),
		newSymbolConsumer[*types.Measurement](symbol, SourceAudit),
	}
	symbol.DecisionConsumers = []*transport.Consumer[Decision]{
		newSymbolConsumer[Decision](symbol, SourceAudit),
	}
	symbol.PositionConsumers = []*transport.Consumer[any]{
		newSymbolConsumer[any](symbol, SourceAudit),
	}
	symbol.GraphConsumers = []*transport.Consumer[*Graph]{
		newSymbolConsumer[*Graph](symbol, SourcePlanner),
		newSymbolConsumer[*Graph](symbol, SourceAudit),
	}
	symbol.CategoryConsumers = []*transport.Consumer[[]Category]{
		newSymbolConsumer[[]Category](symbol, SourceGraph),
		newSymbolConsumer[[]Category](symbol, SourceCognition),
		newSymbolConsumer[[]Category](symbol, SourceAudit),
	}
	symbol.PhaseConsumers = []*transport.Consumer[PhaseReading]{
		newSymbolConsumer[PhaseReading](symbol, SourceGraph),
		newSymbolConsumer[PhaseReading](symbol, SourceAudit),
	}
	symbol.CognitionConsumers = []*transport.Consumer[Cognition]{
		newSymbolConsumer[Cognition](symbol, SourceGraph),
		newSymbolConsumer[Cognition](symbol, SourceAudit),
	}
	symbol.ResonanceConsumers = []*transport.Consumer[any]{
		newSymbolConsumer[any](symbol, SourceCausal),
		newSymbolConsumer[any](symbol, SourceGraph),
		newSymbolConsumer[any](symbol, SourceAudit),
	}
	symbol.CausalConsumers = []*transport.Consumer[map[string]any]{
		newSymbolConsumer[map[string]any](symbol, SourceGraph),
		newSymbolConsumer[map[string]any](symbol, SourceCausal),
		newSymbolConsumer[map[string]any](symbol, SourceAudit),
	}
	symbol.tickers = transport.NewMapReduce(symbol.TickerConsumers, nil, nil)
	symbol.trades = transport.NewMapReduce(symbol.TradeConsumers, nil, nil)
	symbol.level3 = transport.NewMapReduce(symbol.Level3Consumers, nil, nil)
	symbol.executions = transport.NewMapReduce(symbol.ExecutionConsumers, nil, nil)
	symbol.Measurements = transport.NewMapReduce(symbol.MeasurementConsumers, nil, nil)
	symbol.Decisions = transport.NewMapReduce(symbol.DecisionConsumers, nil, nil)
	symbol.Positions = transport.NewMapReduce(symbol.PositionConsumers, nil, nil)
	symbol.Graphs = transport.NewMapReduce(symbol.GraphConsumers, nil, nil)
	symbol.Categories = transport.NewMapReduce(symbol.CategoryConsumers, nil, nil)
	symbol.Phase = transport.NewMapReduce(symbol.PhaseConsumers, nil, nil)
	symbol.Cognition = transport.NewMapReduce(symbol.CognitionConsumers, nil, nil)
	symbol.Resonance = transport.NewMapReduce(symbol.ResonanceConsumers, nil, nil)
	symbol.Causal = transport.NewMapReduce(symbol.CausalConsumers, nil, nil)

	return symbol
}

func (symbol *Symbol) setNotify(notifyFn func(SourceType, *Symbol)) {
	symbol.notify = notifyFn
}

func newSymbolConsumer[T any](
	symbol *Symbol, source SourceType,
) *transport.Consumer[T] {
	consumer := transport.NewConsumer[T](string(source), func() {
		if symbol.notify != nil {
			symbol.notify(source, symbol)
		}
	})

	if source == SourceAudit {
		return consumer.Coalesce()
	}

	return consumer
}

/*
AppendTicker routes a ticker only to the signal owners
selected by the streaming topology.
*/
func (symbol *Symbol) AppendTicker(ticker kraken.TickerData) {
	symbol.tickers.Push(ticker)
}

func (symbol *Symbol) HasTickersFor(
	consumer *transport.Consumer[kraken.TickerData],
) bool {
	return symbol.tickers.Length(consumer) > 0
}

func (symbol *Symbol) HasTradesFor(
	consumer *transport.Consumer[kraken.TradeData],
) bool {
	return symbol.trades.Length(consumer) > 0
}

func (symbol *Symbol) HasLevel3For(
	consumer *transport.Consumer[kraken.Level3Data],
) bool {
	return symbol.level3.Length(consumer) > 0
}

func (symbol *Symbol) HasExecutionsFor(
	consumer *transport.Consumer[kraken.ExecutionData],
) bool {
	return symbol.executions.Length(consumer) > 0
}

/*
AppendTrade routes a trade only to the signal owners
selected by the streaming topology.
*/
func (symbol *Symbol) AppendTrade(trade kraken.TradeData) {
	symbol.trades.Push(trade)
}

/*
AppendLevel3 retains one accepted order-identity frame for book-geometry
readers. Toxicity still observes every fill and delete on its own FIFO.
*/
func (symbol *Symbol) AppendLevel3(level3 kraken.Level3Data) {
	symbol.level3.Push(level3)
}

func (symbol *Symbol) AppendExecution(execution kraken.ExecutionData) {
	symbol.executions.Push(execution)
}

/*
MarketExecutions drains this source's execution queue up to an event-time cut
taken when the drain starts, on the same terms as MarketTickers.
*/
func (symbol *Symbol) MarketExecutions(
	consumer *transport.Consumer[kraken.ExecutionData],
) iter.Seq[kraken.ExecutionData] {
	cut := time.Now().UTC()

	return symbol.executions.Drain(consumer, func(execution kraken.ExecutionData) bool {
		if execution.Timestamp.After(cut) {
			return false
		}

		return true
	})
}

/*
AppendMeasurement pushes one measured measurement — stamping the owning
symbol's tick at the boundary — and routes it to every solver cursor that
consumes it. Signals project their numeric Frame output into the nomagique
Measurement shape via AddMetrics and push that shape here.
*/
func (symbol *Symbol) AppendMeasurement(measurement *types.Measurement) {
	if measurement == nil {
		panic("symbol: measurement required")
	}

	if measurement.Symbol != "" && measurement.Symbol != symbol.Symbol {
		panic("symbol: measurement belongs to " + measurement.Symbol +
			", not " + symbol.Symbol)
	}

	measurement.Symbol = symbol.Symbol
	measurement.Tick = symbol.Tick
	symbol.Measurements.Push(measurement)

	if symbol.ui == nil || symbol.Symbol != Focus() {
		return
	}

	symbol.ui.Push(&wire.FrameT{
		Type: wire.FrameMeasurementsFrame,
		Value: &wire.MeasurementsFrameT{
			Rows: []*wire.MeasurementT{measurementWire(symbol.Symbol, measurement)},
		},
	})
}

func measurementWire(
	symbolName string, measurement *types.Measurement,
) *wire.MeasurementT {
	metrics := make([]*wire.MetricT, 0, len(measurement.Metrics))
	metricNames := make([]string, 0, len(measurement.Metrics))

	for name := range measurement.Metrics {
		metricNames = append(metricNames, name)
	}

	sort.Strings(metricNames)

	for _, name := range metricNames {
		metric := measurement.Metrics[name]

		if metric == nil {
			continue
		}

		encoded := &wire.MetricT{
			Name: name,
			Raw:  metric.Raw,
			Unit: string(metric.Unit),
		}

		if metric.Normalized != nil {
			encoded.Normalized = *metric.Normalized
			encoded.HasNormalized = true
		}

		metrics = append(metrics, encoded)
	}

	metadata := make([]*wire.NamedNumberT, 0, len(measurement.Metadata))
	metadataNames := make([]string, 0, len(measurement.Metadata))

	for name := range measurement.Metadata {
		metadataNames = append(metadataNames, name)
	}

	sort.Strings(metadataNames)

	for _, name := range metadataNames {
		metadata = append(metadata, &wire.NamedNumberT{
			Name:  name,
			Value: measurement.Metadata[name],
		})
	}

	return &wire.MeasurementT{
		Id:               measurement.ID,
		Source:           measurement.Source,
		Symbol:           symbolName,
		Tick:             measurement.Tick,
		Peer:             measurement.Peer,
		At:               timestampNano(measurement.At),
		ObservedFrom:     timestampNano(measurement.ObservedFrom),
		Horizon:          int64(measurement.Horizon),
		PeerAt:           timestampNano(measurement.PeerAt),
		PeerObservedFrom: timestampNano(measurement.PeerObservedFrom),
		Maturity:         measurement.Maturity,
		Metrics:          metrics,
		Metadata:         metadata,
	}
}

func timestampNano(at time.Time) int64 {
	if at.IsZero() {
		return 0
	}

	return at.UnixNano()
}

/*
CheckpointState materializes the drained decision artifacts needed to audit a
completed thesis lifecycle. Each MapReduce column is drained on a dedicated
audit cursor so the checkpoint does not collide with live solver consumption.
*/
func (symbol *Symbol) CheckpointState() any {
	checkpoint := struct {
		ID         SymbolID `json:"id,omitempty"`
		Symbol     string   `json:"symbol"`
		Status     Status   `json:"status"`
		Tick       int64    `json:"tick"`
		Decisions  any      `json:"decisions"`
		Graphs     any      `json:"graphs"`
		Categories any      `json:"categories"`
		Phase      any      `json:"phase"`
		Cognition  any      `json:"cognition"`
		Resonance  any      `json:"resonance"`
		Causal     any      `json:"causal"`
	}{
		ID:     symbol.ID,
		Symbol: symbol.Symbol,
		Status: symbol.Status,
		Tick:   symbol.Tick,
		Decisions: drainedLatest(
			symbol.Decisions, symbol.DecisionConsumers[0],
		),
		Graphs: drainedLatest(
			symbol.Graphs, symbol.GraphConsumers[GraphConsumerAudit],
		),
		Categories: drainedLatestCategories(symbol),
		Phase: drainedLatest(
			symbol.Phase, symbol.PhaseConsumers[StateConsumerAudit],
		),
		Cognition: drainedLatest(
			symbol.Cognition, symbol.CognitionConsumers[StateConsumerAudit],
		),
		Resonance: drainedLatest(
			symbol.Resonance, symbol.ResonanceConsumers[ResonanceConsumerAudit],
		),
		Causal: drainedLatest(
			symbol.Causal, symbol.CausalConsumers[CausalConsumerAudit],
		),
	}

	return checkpoint
}

func (symbol *Symbol) HasGraphInputs() bool {
	return symbol.Measurements.Length(
		symbol.MeasurementConsumers[MeasurementConsumerGraph],
	) > 0 || symbol.Categories.Length(
		symbol.CategoryConsumers[CategoryConsumerGraph],
	) > 0 || symbol.Resonance.Length(
		symbol.ResonanceConsumers[ResonanceConsumerGraph],
	) > 0 || symbol.Cognition.Length(
		symbol.CognitionConsumers[StateConsumerStage],
	) > 0 || symbol.Causal.Length(
		symbol.CausalConsumers[CausalConsumerGraph],
	) > 0 || symbol.Phase.Length(
		symbol.PhaseConsumers[StateConsumerStage],
	) > 0
}

/*
HasPendingWork reports whether a deferred derived stage retains input on its
own registered cursor. It never inspects audit cursors or unrelated readers.
*/
func (symbol *Symbol) HasPendingWork(source SourceType) bool {
	if symbol == nil {
		return false
	}

	switch source {
	case SourceCorrelation:
		return symbol.HasTickersFor(
			symbol.TickerConsumers[TickerConsumerCorrelation],
		)
	case SourceCVD:
		return symbol.HasTradesFor(symbol.TradeConsumers[TradeConsumerCVD])
	case SourceDepthFlow:
		return symbol.HasLevel3For(
			symbol.Level3Consumers[Level3ConsumerDepthFlow],
		)
	case SourceExhaustion:
		return symbol.HasTradesFor(
			symbol.TradeConsumers[TradeConsumerExhaustion],
		)
	case SourceHawkes:
		return symbol.HasTradesFor(symbol.TradeConsumers[TradeConsumerHawkes])
	case SourceLeadLag:
		return symbol.HasTickersFor(
			symbol.TickerConsumers[TickerConsumerLeadLag],
		)
	case SourceLiquidity:
		return symbol.HasTickersFor(
			symbol.TickerConsumers[TickerConsumerLiquidity],
		)
	case SourcePumpDump:
		return symbol.HasTickersFor(
			symbol.TickerConsumers[TickerConsumerPumpDump],
		) || symbol.HasTradesFor(
			symbol.TradeConsumers[TradeConsumerPumpDump],
		) || symbol.HasLevel3For(
			symbol.Level3Consumers[Level3ConsumerPumpDump],
		)
	case SourceSentiment:
		return symbol.HasTickersFor(
			symbol.TickerConsumers[TickerConsumerSentiment],
		)
	case SourceToxicity:
		return symbol.HasLevel3For(
			symbol.Level3Consumers[Level3ConsumerToxicity],
		)
	case SourceResonance:
		return symbol.HasTickersFor(
			symbol.TickerConsumers[TickerConsumerResonance],
		)
	case SourceCategory:
		return symbol.Measurements.Length(
			symbol.MeasurementConsumers[MeasurementConsumerCategory],
		) > 0
	case SourceManifold:
		return symbol.Measurements.Length(
			symbol.MeasurementConsumers[MeasurementConsumerManifold],
		) > 0
	case SourceCausal:
		return symbol.Resonance.Length(
			symbol.ResonanceConsumers[ResonanceConsumerCausal],
		) > 0
	case SourceCognition:
		return symbol.Categories.Length(
			symbol.CategoryConsumers[CategoryConsumerCognition],
		) > 0
	case SourceGraph:
		return symbol.HasGraphInputs()
	case SourcePlanner:
		return symbol.Graphs.Length(
			symbol.GraphConsumers[GraphConsumerPlanner],
		) > 0
	default:
		panic("symbol: source cannot be deferred: " + string(source))
	}
}

func drainedLatest[T any](
	mapReduce *transport.MapReduce[T], consumer *transport.Consumer[T],
) []T {
	var latest T
	found := false

	for item := range mapReduce.Drain(consumer, nil) {
		latest = item
		found = true
	}

	if !found {
		return nil
	}

	return []T{latest}
}

func drainedLatestCategories(symbol *Symbol) []Category {
	var latest []Category

	for batch := range symbol.Categories.Drain(
		symbol.CategoryConsumers[CategoryConsumerAudit], nil,
	) {
		latest = batch
	}

	return latest
}

/*
QueueDepths reports the current pending item count of every per-symbol stage
buffer. Audit cursors are omitted because they retain only the newest artifact
for checkpoints and would otherwise dominate live pressure. The keys are stable
wire names the diagnostics page renders as labeled lanes; a nil queue is
reported with a zero entry rather than omitted so the consumer can render a
full row even before the producer first writes.
*/
func (symbol *Symbol) QueueDepths() map[string]uint64 {
	out := make(map[string]uint64, 12)

	if symbol == nil {
		return out
	}

	if symbol.tickers != nil {
		out["tickers"] = symbol.tickers.Length()
	}

	if symbol.trades != nil {
		out["trades"] = symbol.trades.Length()
	}

	if symbol.level3 != nil {
		out["level3"] = symbol.level3.Length()
	}

	if symbol.executions != nil {
		out["executions"] = symbol.executions.Length()
	}

	if symbol.Measurements != nil {
		out["measurements"] = symbol.Measurements.Length(
			symbol.MeasurementConsumers[MeasurementConsumerCategory],
			symbol.MeasurementConsumers[MeasurementConsumerManifold],
			symbol.MeasurementConsumers[MeasurementConsumerGraph],
		)
	}

	if symbol.Decisions != nil {
		out["decisions"] = symbol.Decisions.Length(
			symbol.DecisionConsumers[0],
		)
	}

	if symbol.Positions != nil {
		out["positions"] = symbol.Positions.Length(
			symbol.PositionConsumers[0],
		)
	}

	if symbol.Graphs != nil {
		out["graphs"] = symbol.Graphs.Length(
			symbol.GraphConsumers[GraphConsumerPlanner],
		)
	}

	if symbol.Categories != nil {
		out["categories"] = symbol.Categories.Length(
			symbol.CategoryConsumers[CategoryConsumerGraph],
			symbol.CategoryConsumers[CategoryConsumerCognition],
		)
	}

	if symbol.Phase != nil {
		out["phase"] = symbol.Phase.Length(
			symbol.PhaseConsumers[StateConsumerStage],
		)
	}

	if symbol.Cognition != nil {
		out["cognition"] = symbol.Cognition.Length(
			symbol.CognitionConsumers[StateConsumerStage],
		)
	}

	if symbol.Resonance != nil {
		out["resonance"] = symbol.Resonance.Length(
			symbol.ResonanceConsumers[ResonanceConsumerCausal],
			symbol.ResonanceConsumers[ResonanceConsumerGraph],
		)
	}

	if symbol.Causal != nil {
		out["causal"] = symbol.Causal.Length(
			symbol.CausalConsumers[CausalConsumerGraph],
			symbol.CausalConsumers[CausalConsumerCausal],
		)
	}

	return out
}

/*
MarketTickers drains this source's ticker queue up to an event-time cut taken
when the drain starts. Ingress can outpace the reader, so a drain that chased
queue emptiness would never end under sustained load; rows stamped after the
cut are processed one last time and then left for the next pass.
*/
func (symbol *Symbol) MarketTickers(
	consumer *transport.Consumer[kraken.TickerData],
) iter.Seq[kraken.TickerData] {
	cut := time.Now().UTC()

	return symbol.tickers.Drain(consumer, func(ticker kraken.TickerData) bool {
		if ticker.Timestamp.After(cut) {
			return false
		}

		return true
	})
}

/*
MarketTrades drains this source's trade queue up to an event-time cut taken
when the drain starts, on the same terms as MarketTickers.
*/
func (symbol *Symbol) MarketTrades(
	consumer *transport.Consumer[kraken.TradeData],
) iter.Seq[kraken.TradeData] {
	cut := time.Now().UTC()

	return symbol.trades.Drain(consumer, func(trade kraken.TradeData) bool {
		if trade.Timestamp.After(cut) {
			return false
		}

		return true
	})
}

/*
MarketLevel3 drains this source's accepted order-identity frames in transport
order, up to an event-time cut taken when the drain starts, on the same terms
as MarketTickers.
*/
func (symbol *Symbol) MarketLevel3(
	consumer *transport.Consumer[kraken.Level3Data],
) iter.Seq[kraken.Level3Data] {
	cut := time.Now().UTC()

	return symbol.level3.Drain(consumer, func(level3 kraken.Level3Data) bool {
		if level3.Timestamp.After(cut) {
			return false
		}

		return true
	})
}

func (symbol *Symbol) MarketMeasurements(
	consumer *transport.Consumer[*types.Measurement],
) iter.Seq[*types.Measurement] {
	cut := time.Now().UTC()

	return symbol.Measurements.Drain(consumer, func(measurement *types.Measurement) bool {
		if measurement == nil {
			return true
		}

		return measurement.At.Before(cut) || measurement.At.Equal(cut)
	})
}

/*
MarketPhase drains this symbol's universe phase sweep to the consuming stage.
*/
func (symbol *Symbol) MarketPhase(
	consumer *transport.Consumer[PhaseReading],
) iter.Seq[PhaseReading] {
	return symbol.Phase.Drain(consumer, func(_ PhaseReading) bool {
		return true
	})
}

/*
MarketCognition drains this symbol's cognition readings to the consuming stage.
*/
func (symbol *Symbol) MarketCognition(
	consumer *transport.Consumer[Cognition],
) iter.Seq[Cognition] {
	return symbol.Cognition.Drain(consumer, func(_ Cognition) bool {
		return true
	})
}

/*
MarketCategories drains this symbol's category batches to the consuming stage.
*/
func (symbol *Symbol) MarketCategories(
	consumer *transport.Consumer[[]Category],
) iter.Seq[[]Category] {
	return symbol.Categories.Drain(consumer, func(_ []Category) bool {
		return true
	})
}

/*
MarketResonance drains this symbol's resonance artifacts to the consuming stage.
*/
func (symbol *Symbol) MarketResonance(
	consumer *transport.Consumer[any],
) iter.Seq[any] {
	return symbol.Resonance.Drain(consumer, func(_ any) bool {
		return true
	})
}

/*
MarketCausal drains this symbol's causal artifacts to the consuming stage.
*/
func (symbol *Symbol) MarketCausal(
	consumer *transport.Consumer[map[string]any],
) iter.Seq[map[string]any] {
	return symbol.Causal.Drain(consumer, func(_ map[string]any) bool {
		return true
	})
}

/*
MarketGraphs drains this symbol's lifecycle graphs to the consuming stage.
*/
func (symbol *Symbol) MarketGraphs(
	consumer *transport.Consumer[*Graph],
) iter.Seq[*Graph] {
	return symbol.Graphs.Drain(consumer, func(_ *Graph) bool {
		return true
	})
}
