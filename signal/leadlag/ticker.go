package leadlag

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/nomagique/adaptive"
	"github.com/theapemachine/symm/nomagique/algo"
	"github.com/theapemachine/symm/nomagique/core"
	nmcorrelation "github.com/theapemachine/symm/nomagique/correlation"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/logic"
	"github.com/theapemachine/symm/nomagique/temporal"
	"github.com/theapemachine/symm/nomagique/transport"
	"github.com/theapemachine/symm/types"
)

/*
Signal is the asynchronous price-path lead-lag instrument. It composes its
per-symbol Ticker entity in its constructor and exposes the canonical signal
structure: Constructor, Name, Error, Step, Close. It satisfies
nomagique/runtime.Node[*types.Envelope], writing its projected Measurement
into the envelope's LeadLag field.
*/
type Signal struct {
	ctx    context.Context
	cancel context.CancelFunc
	err    error
	ticker *Ticker
}

/*
NewSignal composes the Ticker entity.
*/
func NewSignal(ctx context.Context) *Signal {
	ctx, cancel := context.WithCancel(ctx)

	return &Signal{
		ctx:    ctx,
		cancel: cancel,
		ticker: NewTicker(),
	}
}

func (signal *Signal) Name() string { return "leadlag" }

func (signal *Signal) Error() error { return signal.err }

func (signal *Signal) Step(envelope *types.Envelope) *types.Envelope {
	if signal.err != nil {
		errnie.Error(signal.Close())
		return nil
	}

	/*
		A signal observes exactly the envelope kind it consumes. Stepping on any
		other kind hands the estimator a zero-valued observation, which it
		correctly rejects — and that rejection becomes a Measurement carrying an
		Err. data.Lift discards the WHOLE frame on the first failed measurement,
		so one signal stepped out of turn erased every other signal's metrics
		from the same envelope, and no advisor could ever assemble a complete
		feature group.
	*/
	if envelope.TypeID != types.EnvelopeTicker {
		return envelope
	}

	envelope.LeadLag = signal.ticker.Step(envelope.TickerData)

	return envelope
}

func (signal *Signal) Close() error {
	if signal.cancel != nil {
		signal.cancel()
	}

	return signal.ticker.Close()
}

type pricePath struct {
	graph   core.Primitive
	reading nmcorrelation.PathReading
}

type Ticker struct {
	search     core.Primitive
	fisher     core.Primitive
	projection *data.Projection
	paths      map[string]*pricePath
	pipelines  map[[2]string]*pipeline
	finite     core.Primitive
}

func NewTicker() *Ticker {
	return &Ticker{
		search:     nmcorrelation.NewLeadLag(algo.NewHayashiYoshida()),
		fisher:     nmcorrelation.NewFisher(),
		projection: lagProjection(),
		paths:      make(map[string]*pricePath),
		pipelines:  make(map[[2]string]*pipeline),
		finite:     logic.NewFinite(),
	}
}

func (ticker *Ticker) Close() error { return nil }

func (ticker *Ticker) Step(event kraken.TickerData) *data.Measurement[float64] {
	if event.Last == nil {
		return &data.Measurement[float64]{Err: fmt.Errorf("leadlag: ticker requires a last price")}
	}
	last := event.Last.Float64()

	if !finiteHolds(ticker.finite, last) || last < 0 {
		return &data.Measurement[float64]{Err: fmt.Errorf("leadlag: finite non-negative last price required")}
	}
	m := data.NewMeasurement[float64]("leadlag", nil)
	m.Label, m.At, m.From = event.Symbol, event.Timestamp, event.Timestamp
	m.Metadata = map[string]float64{data.MetadataSupport: 0}
	if last == 0 {
		m.Provenance = map[string]string{"last_trade_price_state": "unobserved"}
		m.Finalize()
		return m
	}

	focal := ticker.paths[event.Symbol]
	if focal == nil {
		focal = &pricePath{graph: nmcorrelation.NewPath(adaptive.NewWindow())}
		ticker.paths[event.Symbol] = focal
	}
	readingEval := transport.NewEvaluate(focal.graph)
	var reading nmcorrelation.PathReading

	for out := range readingEval.Next(transport.NewValues(temporal.Price{At: event.Timestamp.UnixNano(), Value: last}).Next(nil)) {
		reading = *(*nmcorrelation.PathReading)(out)
	}

	err := readingEval.Error()
	if err != nil {
		m.Err = err
		return m
	}
	if !reading.Accepted {
		m.Provenance = map[string]string{"event_time_state": "regressed"}
		m.Finalize()
		return m
	}
	focal.reading = reading
	putMetric(m, "last_price", last, data.UnitRate, data.TimescaleInstantaneous)
	putMetric(m, "observation_count", reading.Count, data.UnitCount, data.TimescaleInstantaneous)
	peers := make([]string, 0, len(ticker.paths))
	for symbol := range ticker.paths {
		if symbol != event.Symbol {
			peers = append(peers, symbol)
		}
	}
	sort.Strings(peers)
	var selected *pipeline
	var selectedPair nmcorrelation.LeadLagReading
	var selectedPeer string
	for _, symbol := range peers {
		pairEval := transport.NewEvaluate(ticker.search)
		var pair nmcorrelation.LeadLagReading

		for out := range pairEval.Next(transport.NewValues(nmcorrelation.LagProfileInput{
			Left: reading.Observations, Right: ticker.paths[symbol].reading.Observations,
		}).Next(nil)) {
			pair = *(*nmcorrelation.LeadLagReading)(out)
		}

		err := pairEval.Error()
		if err != nil {
			m.Err = err
			return m
		}
		if !pair.Defined {
			continue
		}
		key := [2]string{event.Symbol, symbol}
		built := ticker.pipelines[key]
		if built == nil {
			built = newPipeline()
			ticker.pipelines[key] = built
		}
		built.Observe(pairObservation{Lag: pair.X, AbsoluteGain: pair.AbsoluteGain, Correlation: pair.Correlation}, event.Timestamp.UnixNano())
		selected, selectedPair, selectedPeer = built, pair, symbol
	}
	if selected == nil {
		m.Finalize()
		return m
	}
	fisherEval := transport.NewEvaluate(ticker.fisher)
	var fisher nmcorrelation.FisherReading

	for out := range fisherEval.Next(transport.NewValues(nmcorrelation.FisherSample{
		Correlation: selectedPair.Correlation, Support: selectedPair.Support, SearchCount: selectedPair.SearchCount,
	}).Next(nil)) {
		fisher = *(*nmcorrelation.FisherReading)(out)
	}

	err = fisherEval.Error()
	if err != nil {
		m.Err = err
		return m
	}
	resolution := selectedPair.Spacing * 1e-9
	progress := selected.project(selectedPair, fisher, resolution, selectedPair.Span*resolution)
	ticker.projection.Identity = func() (string, string, time.Time, time.Time) {
		return m.ID, event.Symbol, event.Timestamp, time.Unix(0, reading.From)
	}
	resultEval := transport.NewEvaluate(ticker.projection)
	var result *data.Measurement[float64]

	for out := range resultEval.Next(transport.NewValues(progress).Next(nil)) {
		result = *(**data.Measurement[float64])(out)
	}

	err = resultEval.Error()

	if err != nil {
		m.Err = err
		return m
	}
	result.Provenance = map[string]string{"peer": selectedPeer, "pair_diagnostics_selection": "last_defined_peer_lexicographic"}
	putMetric(result, "last_price", last, data.UnitRate, data.TimescaleInstantaneous)
	putMetric(result, "observation_count", reading.Count, data.UnitCount, data.TimescaleInstantaneous)
	return result
}

func putMetric(m *data.Measurement[float64], label string, raw float64, unit data.Unit, scale data.Timescale) {
	m.Metrics[label] = data.Metric[float64]{Label: label, Raw: raw, Unit: unit, Timescale: scale}
}

/*
finiteHolds reports whether one value passes the Finite primitive.
*/
func finiteHolds(finite core.Primitive, value float64) bool {
	holdsEval := transport.NewEvaluate(finite)
	var holds bool

	for out := range holdsEval.Next(transport.NewValues(value).Next(nil)) {
		holds = *(*bool)(out)
	}

	return holdsEval.Error() == nil && holds
}
