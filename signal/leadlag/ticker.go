package leadlag

import (
	"fmt"
	"sort"
	"time"

	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/nomagique/adaptive"
	"github.com/theapemachine/symm/nomagique/algo"
	nmcorrelation "github.com/theapemachine/symm/nomagique/correlation"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/equation"
	"github.com/theapemachine/symm/nomagique/logic"
	"github.com/theapemachine/symm/nomagique/transport"
)

type pricePath struct {
	graph   *nmcorrelation.Path
	reading nmcorrelation.PathReading
}

type Ticker struct {
	search     *nmcorrelation.LeadLag
	fisher     *nmcorrelation.Fisher
	projection *data.Projection
	paths      map[string]*pricePath
	pipelines  map[[2]string]*pipeline
	finite     *logic.Finite[float64]
}

func NewTicker() *Ticker {
	return &Ticker{
		search:     nmcorrelation.NewLeadLag(algo.NewHayashiYoshida()),
		fisher:     nmcorrelation.NewFisher(),
		projection: lagProjection(),
		paths:      make(map[string]*pricePath),
		pipelines:  make(map[[2]string]*pipeline),
		finite:     logic.NewFinite[float64](),
	}
}

func (ticker *Ticker) Close() error { return nil }

func (ticker *Ticker) Step(event kraken.TickerData) *data.Measurement[float64] {
	if event.Last == nil {
		return &data.Measurement[float64]{Err: fmt.Errorf("leadlag: ticker requires a last price")}
	}
	last := event.Last.Float64()

	if !ticker.finite.Holds(last) || last < 0 {
		return &data.Measurement[float64]{Err: fmt.Errorf("leadlag: finite non-negative last price required")}
	}
	m := data.NewMeasurement[float64](event.Symbol+":leadlag:"+event.Timestamp.Format(time.RFC3339Nano), event.Symbol, "leadlag", event.Timestamp, event.Timestamp)
	m.Metadata = map[string]float64{data.MetadataSupport: 0}
	if last == 0 {
		m.Provenance = map[string]string{"last_trade_price_state": "unobserved"}
		m.Finalize()
		return m
	}

	focal := ticker.paths[event.Symbol]
	if focal == nil {
		focal = &pricePath{graph: adaptive.NewPath(adaptive.NewWindow())}
		ticker.paths[event.Symbol] = focal
	}
	reading, err := transport.Evaluate(focal.graph, transport.Values(equation.Price{At: event.Timestamp.UnixNano(), Value: last}))
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
		pair, err := transport.Evaluate(ticker.search, transport.Values(equation.LagProfileInput{
			Left: reading.Observations, Right: ticker.paths[symbol].reading.Observations,
		}))
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
	fisher, err := transport.Evaluate(ticker.fisher, transport.Values(nmcorrelation.FisherSample{
		Correlation: selectedPair.Correlation, Support: selectedPair.Support, SearchCount: selectedPair.SearchCount,
	}))
	if err != nil {
		m.Err = err
		return m
	}
	resolution := selectedPair.Spacing * 1e-9
	progress := selected.project(selectedPair, fisher, resolution, selectedPair.Span*resolution)
	ticker.projection.Identity = func() (string, string, time.Time, time.Time) {
		return m.ID, event.Symbol, event.Timestamp, time.Unix(0, reading.From)
	}
	result := ticker.projection.Project(progress)
	if result.Err != nil {
		return result
	}
	result.Provenance = map[string]string{"peer": selectedPeer, "pair_diagnostics_selection": "last_defined_peer_lexicographic"}
	putMetric(result, "last_price", last, data.UnitRate, data.TimescaleInstantaneous)
	putMetric(result, "observation_count", reading.Count, data.UnitCount, data.TimescaleInstantaneous)
	return result
}

func putMetric(m *data.Measurement[float64], label string, raw float64, unit data.Unit, scale data.Timescale) {
	m.PutMetric(data.Metric[float64]{Label: label, Raw: raw, Unit: unit, Timescale: scale})
}
