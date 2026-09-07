package correlation

import (
	"fmt"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/nomagique/adaptive"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/transport"
	"math"
	"sort"
	"sync"
	"time"
)

type pricePath struct {
	graph  core.Primitive
	record map[string]core.Primitive
}

// Ticker owns each symbol's timestamped input path. Pairwise estimates use the
// actual asynchronous paths; cohort histories advance once per focal event.
type Ticker struct {
	mutex     sync.Mutex
	Relations Relations
	paths     map[string]*pricePath
	pipelines map[string]*pipeline
}

func NewTicker() *Ticker {
	return &Ticker{paths: make(map[string]*pricePath), pipelines: make(map[string]*pipeline)}
}
func (ticker *Ticker) Close() error { return nil }

func (ticker *Ticker) Step(event kraken.TickerData) *data.Measurement[float64] {
	if event.Last == nil {
		return &data.Measurement[float64]{Err: fmt.Errorf("correlation: ticker requires a last price")}
	}
	last := event.Last.Float64()
	if last < 0 || math.IsNaN(last) || math.IsInf(last, 0) {
		return &data.Measurement[float64]{Err: fmt.Errorf("correlation: finite non-negative last price required")}
	}
	m := data.NewMeasurement[float64](event.Symbol+":correlation:"+event.Timestamp.Format(time.RFC3339Nano), event.Symbol, "correlation", event.Timestamp, event.Timestamp)
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
	decoder := core.NewDecoder(fields)
	accepted := core.Decode[bool](decoder, "accepted")
	if decoder.Error() != nil {
		m.Err = decoder.Error()
		return m
	}
	if !accepted {
		m.Provenance = map[string]string{"event_time_state": "regressed"}
		m.Finalize()
		return m
	}
	focal.record = fields
	count := core.Decode[float64](decoder, "count")
	observations := core.Decode[[]core.Primitive](decoder, "observations")
	from := core.Decode[int64](decoder, "from")
	if decoder.Error() != nil {
		m.Err = decoder.Error()
		return m
	}
	putMetric(m, "last_price", last, data.UnitRate, data.TimescaleInstantaneous)
	putMetric(m, "observation_count", count, data.UnitCount, data.TimescaleInstantaneous)
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
	admitted := []core.Primitive{}
	var selected map[string]core.Primitive
	selectedSymbol := ""
	for _, symbol := range peers {
		peer := ticker.paths[symbol]
		peerDecoder := core.NewDecoder(peer.record)
		right := core.Decode[[]core.Primitive](peerDecoder, "observations")
		rightAt := core.Decode[int64](peerDecoder, "to")
		if peerDecoder.Error() != nil {
			m.Err = peerDecoder.Error()
			return m
		}
		pair, err := transport.Evaluate[map[string]core.Primitive](built.pairwise, core.Record(map[string]any{"left": observations, "right": right}))
		if err != nil {
			m.Err = err
			return m
		}
		if err = ticker.Relations.observe(event.Symbol, symbol, pair, event.Timestamp.UnixNano(), rightAt); err != nil {
			m.Err = err
			return m
		}
		d := core.NewDecoder(pair)
		defined := core.Decode[bool](d, "defined")
		support := core.Decode[float64](d, "support")
		if d.Error() != nil {
			m.Err = d.Error()
			return m
		}
		if !defined || support < 2 {
			continue
		}
		admitted = append(admitted, core.From(map[string]core.Primitive{"correlation": pair["correlation"], "support": pair["support"], "peer_energy": pair["right_energy_rate"]}))
		selected, selectedSymbol = pair, symbol
	}
	if len(admitted) == 0 {
		m.Finalize()
		return m
	}
	cohort, err := transport.Evaluate[map[string]core.Primitive](built.cohort, core.From(admitted))
	if err != nil {
		m.Err = err
		return m
	}
	progress, err := transport.Evaluate[map[string]core.Primitive](built.progress, core.From(map[string]core.Primitive{"pair": core.From(selected), "cohort": core.From(cohort), "at": core.From(event.Timestamp.UnixNano())}))
	if err != nil {
		m.Err = err
		return m
	}
	built.projection.Identity = func() (string, string, time.Time, time.Time) {
		return m.ID, event.Symbol, event.Timestamp, time.Unix(0, from)
	}
	result := built.projection.Project(progress)
	putMetric(result, "last_price", last, data.UnitRate, data.TimescaleInstantaneous)
	putMetric(result, "observation_count", count, data.UnitCount, data.TimescaleInstantaneous)
	// Pair diagnostics name a deterministic, defined peer. All admitted peers
	// still enter the cohort; an empty final peer cannot overwrite a good pair.
	result.Provenance = map[string]string{"peer": selectedSymbol, "pair_diagnostics_selection": "last_defined_peer_lexicographic"}
	return result
}

func putMetric(m *data.Measurement[float64], label string, raw float64, unit data.Unit, scale data.Timescale) {
	m.PutMetric(data.Metric[float64]{Label: label, Raw: raw, Unit: unit, Timescale: scale})
}
