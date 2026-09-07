package leadlag

import (
	"fmt"
	"math"
	"sort"
	"sync"
	"time"

	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/nomagique/adaptive"
	"github.com/theapemachine/symm/nomagique/algo"
	"github.com/theapemachine/symm/nomagique/core"
	nmcorrelation "github.com/theapemachine/symm/nomagique/correlation"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/transport"
)

type pricePath struct {
	graph  core.Primitive
	record map[string]core.Primitive
}

// Ticker preserves asynchronous per-symbol paths, with causal history owned
// by each ordered pair. There is no cross-market synchronization barrier.
type Ticker struct {
	mutex      sync.Mutex
	search     core.Primitive
	fisher     core.Primitive
	projection *data.Projection
	paths      map[string]*pricePath
	pipelines  map[[2]string]*pipeline
}

func NewTicker() *Ticker {
	return &Ticker{search: nmcorrelation.NewLeadLag(algo.NewHayashiYoshida()), fisher: nmcorrelation.NewFisher(), projection: lagProjection(), paths: make(map[string]*pricePath), pipelines: make(map[[2]string]*pipeline)}
}
func (ticker *Ticker) Close() error { return nil }

func (ticker *Ticker) Step(event kraken.TickerData) *data.Measurement[float64] {
	if event.Last == nil {
		return &data.Measurement[float64]{Err: fmt.Errorf("leadlag: ticker requires a last price")}
	}
	last := event.Last.Float64()
	if last < 0 || math.IsNaN(last) || math.IsInf(last, 0) {
		return &data.Measurement[float64]{Err: fmt.Errorf("leadlag: finite non-negative last price required")}
	}
	m := data.NewMeasurement[float64](event.Symbol+":leadlag:"+event.Timestamp.Format(time.RFC3339Nano), event.Symbol, "leadlag", event.Timestamp, event.Timestamp)
	m.Metadata = map[string]float64{data.MetadataSupport: 0}
	if last == 0 {
		m.Provenance = map[string]string{"last_trade_price_state": "unobserved"}
		m.Finalize()
		return m
	}
	ticker.mutex.Lock()
	defer ticker.mutex.Unlock()
	focal := ticker.paths[event.Symbol]
	if focal == nil {
		focal = &pricePath{graph: adaptive.NewPath(adaptive.NewWindow())}
		ticker.paths[event.Symbol] = focal
	}
	fields, err := transport.Evaluate[map[string]core.Primitive](focal.graph, core.Record(map[string]any{"at": event.Timestamp.UnixNano(), "value": last}))
	if err != nil {
		m.Err = err
		return m
	}
	d := core.NewDecoder(fields)
	accepted := core.Decode[bool](d, "accepted")
	if d.Error() != nil {
		m.Err = d.Error()
		return m
	}
	if !accepted {
		m.Provenance = map[string]string{"event_time_state": "regressed"}
		m.Finalize()
		return m
	}
	focal.record = fields
	count := core.Decode[float64](d, "count")
	left := core.Decode[[]core.Primitive](d, "observations")
	from := core.Decode[int64](d, "from")
	if d.Error() != nil {
		m.Err = d.Error()
		return m
	}
	putMetric(m, "last_price", last, data.UnitRate, data.TimescaleInstantaneous)
	putMetric(m, "observation_count", count, data.UnitCount, data.TimescaleInstantaneous)
	peers := make([]string, 0, len(ticker.paths))
	for symbol := range ticker.paths {
		if symbol != event.Symbol {
			peers = append(peers, symbol)
		}
	}
	sort.Strings(peers)
	var selected *pipeline
	var selectedPair map[string]core.Primitive
	var selectedPeer string
	for _, symbol := range peers {
		right, err := core.Field[[]core.Primitive](ticker.paths[symbol].record, "observations")
		if err != nil {
			m.Err = err
			return m
		}
		pair, err := transport.Evaluate[map[string]core.Primitive](ticker.search, core.Record(map[string]any{"left": left, "right": right}))
		if err != nil {
			m.Err = err
			return m
		}
		defined, err := core.Field[bool](pair, "defined")
		if err != nil {
			m.Err = err
			return m
		}
		if !defined {
			continue
		}
		key := [2]string{event.Symbol, symbol}
		built := ticker.pipelines[key]
		if built == nil {
			built = newPipeline()
			ticker.pipelines[key] = built
		}
		if err := built.Observe(pair, event.Timestamp.UnixNano()); err != nil {
			m.Err = err
			return m
		}
		selected, selectedPair, selectedPeer = built, pair, symbol
	}

	if selected == nil {
		m.Finalize()
		return m
	}
	fisher, err := transport.Evaluate[map[string]core.Primitive](ticker.fisher, core.From(selectedPair))

	if err != nil {
		m.Err = err
		return m
	}
	decoder := core.NewDecoder(selectedPair)
	resolution := core.Decode[float64](decoder, "spacing") * 1e-9
	span := core.Decode[float64](decoder, "span")

	if err := decoder.Error(); err != nil {
		m.Err = err
		return m
	}
	progress := selected.Fields(selectedPair)
	progress["fisher"] = core.From(fisher)
	progress["resolution"] = core.From(resolution)
	progress["span_seconds"] = core.From(span * resolution)
	ticker.projection.Identity = func() (string, string, time.Time, time.Time) {
		return m.ID, event.Symbol, event.Timestamp, time.Unix(0, from)
	}
	result := ticker.projection.Project(progress)

	if result.Err != nil {
		return result
	}
	result.Provenance = map[string]string{"peer": selectedPeer, "pair_diagnostics_selection": "last_defined_peer_lexicographic"}
	putMetric(result, "last_price", last, data.UnitRate, data.TimescaleInstantaneous)
	putMetric(result, "observation_count", count, data.UnitCount, data.TimescaleInstantaneous)
	return result
}
func putMetric(m *data.Measurement[float64], label string, raw float64, unit data.Unit, scale data.Timescale) {
	m.PutMetric(data.Metric[float64]{Label: label, Raw: raw, Unit: unit, Timescale: scale})
}
