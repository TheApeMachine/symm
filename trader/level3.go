package trader

import (
	"sync"
	"sync/atomic"
	"time"

	"github.com/theapemachine/datura"
	"github.com/theapemachine/datura/structure"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/logic"
	"github.com/theapemachine/symm/types"
)

const (
	level3DefaultPrecision = 8
	level3IdlePoll         = 200 * time.Microsecond
)

/*
Level3 consumes the level3 order-level channel, measures its composed
signals against every row, and drives the manifold analyzer's per-row
physics step. A manifold step is a GPU round trip and can run far longer
than one trading tick, so Level3 owns a dedicated goroutine that drains
its ring at whatever pace the physics can sustain rather than at the
pace Crypto publishes ticks. Crypto's tick loop only ever reads back
whatever measurements have accumulated since its last call, so a slow
symbol's physics step never delays another feed's measurement or the
next published tick. The theses that physics step produces are the
analyzer's output, not the feed's, so Level3 hands each row to
logic.Analyzer.IngestLevel3 and lets the analyzer track them; Crypto
drains those via analyzer.PendingTheses.
*/
type Level3 struct {
	status     atomic.Value
	signals    []types.Signal[any]
	ring       *structure.SPSCRing[[]byte]
	uiHub      chan []byte
	instrument *Instrument
	analyzer   *logic.Analyzer
	book       *Level3Book

	mu           sync.Mutex
	measurements []*types.Measurement
}

/*
NewLevel3 wires the level3 feed to the instrument cache, manifold
analyzer, and local order book that its per-row physics step needs, then
starts the background consumer that owns the ring's drain side.
*/
func NewLevel3(
	signal *Signal,
	uiHub chan []byte,
	instrument *Instrument,
	analyzer *logic.Analyzer,
	book *Level3Book,
) *Level3 {
	level3 := &Level3{
		signals:    signal.Level3,
		ring:       structure.NewSPSCRing[[]byte](8*1024, false),
		uiHub:      uiHub,
		instrument: instrument,
		analyzer:   analyzer,
		book:       book,
	}

	level3.status.Store(types.INITIALIZING)

	go level3.consume()

	return level3
}

func (level3 *Level3) Status() types.Status {
	return level3.status.Load().(types.Status)
}

func (level3 *Level3) On(data []byte) {
	frame := make([]byte, len(data))
	copy(frame, data)

	if !level3.ring.Push(frame) {
		errnie.Error(errnie.Err(
			errnie.UnprocessableContent,
			"trader: level3 ring full",
			nil,
		))
	}
}

/*
Measure drains and returns the measurements the background consumer has
accumulated since the last call. It never touches the ring itself; the
ring's sole consumer is the goroutine started by NewLevel3.
*/
func (level3 *Level3) Measure() ([]*types.Measurement, error) {
	level3.mu.Lock()
	measurements := level3.measurements
	level3.measurements = nil
	level3.mu.Unlock()

	return measurements, nil
}

/*
consume owns the ring's sole Pop side for the lifetime of the feed,
draining and ingesting frames as fast as the manifold physics step
allows. It idles on a short sleep rather than a busy spin when the ring
is empty, and resumes draining at full speed the instant a frame lands.
*/
func (level3 *Level3) consume() {
	for {
		frame := level3.ring.Pop()

		if len(frame) == 0 {
			time.Sleep(level3IdlePoll)
			continue
		}

		level3.ingest(frame)
	}
}

/*
ingest decodes one raw level3 frame into its rows, measures the feed's
composed signals against each row, and drives the manifold analyzer's
per-row physics step, which is the GPU-bound operation this goroutine
exists to keep off Crypto's tick loop.
*/
func (level3 *Level3) ingest(frame []byte) {
	message := kraken.NewLevel3(frame).Data

	if level3.Status() != types.READY && len(message) > 0 {
		level3.status.Store(types.READY)
		errnie.Info("level3 ready")
	}

	measurements := make([]*types.Measurement, 0, len(message))

	for _, row := range message {
		measurements = append(measurements, level3.measure(row)...)
		level3.step(row)
	}

	if len(measurements) == 0 {
		return
	}

	level3.mu.Lock()
	level3.measurements = append(level3.measurements, measurements...)
	level3.mu.Unlock()

	select {
	case level3.uiHub <- datura.Map[any]{
		"measurements": measurements,
	}.Marshal():
	default:
	}
}

/*
measure runs row through the feed's composed signals and stamps the
resulting measurements with the row's mid price. A signal error is
logged and that signal's contribution skipped; it does not abort
measurement of the remaining signals or rows in this frame.
*/
func (level3 *Level3) measure(row kraken.Level3Data) []*types.Measurement {
	results := measureSignals(level3.signals, func(signal types.Signal[any]) ([]*types.Measurement, error) {
		return signal.Measure(row, &types.CrossSection{})
	})

	measurements := make([]*types.Measurement, 0, len(results))

	for _, result := range results {
		if result.err != nil {
			errnie.Error(errnie.Err(
				errnie.UnprocessableContent,
				result.err.Error(),
				result.err,
			))

			continue
		}

		if len(result.measurements) == 0 {
			continue
		}

		price := level3MidPrice(row)

		for _, item := range result.measurements {
			if item.Metrics == nil {
				item.Metrics = map[string]float64{}
			}

			if price > 0 {
				item.Metrics["price"] = price
			}
		}

		measurements = append(measurements, result.measurements...)
	}

	return measurements
}

/*
step applies row to the local order book and advances the manifold
analyzer's field slot for its symbol. The analyzer tracks the resulting
thesis itself (logic.Analyzer.PendingTheses), since a thesis is analysis
output, not a property of this market-data feed.
*/
func (level3 *Level3) step(row kraken.Level3Data) {
	pricePrecision := level3DefaultPrecision
	qtyPrecision := level3DefaultPrecision

	if pair, err := level3.instrument.Pair(row.Symbol); err == nil {
		pricePrecision = pair.PricePrecision
		qtyPrecision = pair.QtyPrecision
	}

	level3.analyzer.IngestLevel3(row, pricePrecision, qtyPrecision, level3.book)
}

func level3MidPrice(row kraken.Level3Data) float64 {
	if len(row.Bids) == 0 || len(row.Asks) == 0 {
		return 0
	}

	return (row.Bids[0].LimitPrice + row.Asks[0].LimitPrice) / 2
}
