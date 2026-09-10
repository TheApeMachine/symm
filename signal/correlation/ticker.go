package correlation

import (
	"fmt"
	"sort"
	"time"

	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/nomagique/adaptive"
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
	Relations Relations
	paths     map[string]*pricePath
	pipelines map[string]*pipeline
	finite    *logic.Finite[float64]
}

func NewTicker() *Ticker {
	return &Ticker{
		paths:     make(map[string]*pricePath),
		pipelines: make(map[string]*pipeline),
		finite:    logic.NewFinite[float64](),
	}
}

func (ticker *Ticker) Close() error { return nil }

func (ticker *Ticker) Step(event kraken.TickerData) *data.Measurement[float64] {
	if event.Last == nil {
		return &data.Measurement[float64]{Err: fmt.Errorf("correlation: ticker requires a last price")}
	}
	last := event.Last.Float64()

	if !ticker.finite.Holds(last) || last < 0 {
		return &data.Measurement[float64]{Err: fmt.Errorf("correlation: finite non-negative last price required")}
	}
	m := data.NewMeasurement[float64](event.Symbol+":correlation:"+event.Timestamp.Format(time.RFC3339Nano), event.Symbol, "correlation", event.Timestamp, event.Timestamp)
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
	built := ticker.pipelines[event.Symbol]
	if built == nil {
		built = newPipeline()
		ticker.pipelines[event.Symbol] = built
	}
	peers := make([]string, 0, len(ticker.paths))
	for symbol := range ticker.paths {
		if symbol != event.Symbol {
			peers = append(peers, symbol)
		}
	}
	sort.Strings(peers)
	admitted := []nmcorrelation.Peer{}
	var selected pairResult
	selectedSymbol := ""
	for _, symbol := range peers {
		peer := ticker.paths[symbol]
		pair, err := built.pair(reading.Observations, peer.reading.Observations)
		if err != nil {
			m.Err = err
			return m
		}
		if err = ticker.Relations.observe(event.Symbol, symbol, pair, event.Timestamp.UnixNano(), peer.reading.To); err != nil {
			m.Err = err
			return m
		}
		if !pair.dependence.Defined || pair.dependence.Support < 2 {
			continue
		}
		admitted = append(admitted, nmcorrelation.Peer{
			Correlation: pair.dependence.Correlation,
			Support:     pair.dependence.Support,
			PeerEnergy:  pair.dependence.RightEnergyRate,
		})
		selected, selectedSymbol = pair, symbol
	}
	if len(admitted) == 0 {
		m.Finalize()
		return m
	}
	cohort, err := built.fold(admitted)
	if err != nil {
		m.Err = err
		return m
	}
	progress, err := built.advance(selected, cohort, event.Timestamp.UnixNano())
	if err != nil {
		m.Err = err
		return m
	}
	built.projection.Identity = func() (string, string, time.Time, time.Time) {
		return m.ID, event.Symbol, event.Timestamp, time.Unix(0, reading.From)
	}
	result := built.projection.Project(progress)
	putMetric(result, "last_price", last, data.UnitRate, data.TimescaleInstantaneous)
	putMetric(result, "observation_count", reading.Count, data.UnitCount, data.TimescaleInstantaneous)
	result.Provenance = map[string]string{"peer": selectedSymbol, "pair_diagnostics_selection": "last_defined_peer_lexicographic"}
	return result
}

func putMetric(m *data.Measurement[float64], label string, raw float64, unit data.Unit, scale data.Timescale) {
	m.PutMetric(data.Metric[float64]{Label: label, Raw: raw, Unit: unit, Timescale: scale})
}
