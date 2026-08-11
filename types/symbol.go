package types

import (
	"fmt"
	"slices"
	"sync"

	"github.com/bytedance/sonic"
	"github.com/theapemachine/symm/kraken"
)

/*
Symbol is an alternative grouping of measurements, and is used in logic that
legitimately needs to fan out measurements by symbol, rather than by source.
It should be kept extremely simple, and lean, and is not an invitation to
start adding additional complexity beyond what is truly earned. Symbols
carry their own readiness state, which is important for the resonance solver,
which needs a stable measurement set to train on.
*/
type Symbol struct {
	measurementMu       sync.RWMutex
	measurementRevision uint64
	tickerMu            sync.RWMutex
	tradeMu             sync.RWMutex
	tickerCursors       *sync.Map
	tradeCursors        *sync.Map
	Symbol              string         `json:"symbol,omitempty"`
	Status              Status         `json:"status,omitempty"`
	Measurements        []*Measurement `json:"measurements,omitempty"`
	Tickers             []kraken.TickerData
	Trades              []kraken.TradeData
	Decisions           *sync.Map `json:"decisions,omitempty"`
	Graphs              *sync.Map `json:"graphs,omitempty"`
	Categories          *sync.Map `json:"categories,omitempty"`
	Phase               *sync.Map `json:"-"`
	Cognition           *sync.Map `json:"-"`
	Resonance           *sync.Map `json:"-"`
	Causal              *sync.Map `json:"-"`
}

/*
NewSymbol creates empty measurement state for one market symbol.
*/
func NewSymbol(symbol string, ui chan []byte) *Symbol {
	return &Symbol{
		Symbol:        symbol,
		Status:        READY,
		Measurements:  make([]*Measurement, 0),
		Tickers:       make([]kraken.TickerData, 0),
		Trades:        make([]kraken.TradeData, 0),
		tickerCursors: &sync.Map{},
		tradeCursors:  &sync.Map{},
		Decisions:     &sync.Map{},
		Graphs:        &sync.Map{},
		Categories:    &sync.Map{},
		Phase:         &sync.Map{},
		Cognition:     &sync.Map{},
		Resonance:     &sync.Map{},
		Causal:        &sync.Map{},
	}
}

func (symbol *Symbol) ResetMarket() {
	symbol.tickerMu.Lock()
	symbol.Tickers = symbol.Tickers[:0]
	symbol.tickerCursors.Clear()
	symbol.tickerMu.Unlock()

	symbol.tradeMu.Lock()
	symbol.Trades = symbol.Trades[:0]
	symbol.tradeCursors.Clear()
	symbol.tradeMu.Unlock()
}

func (symbol *Symbol) AppendTicker(ticker kraken.TickerData) {
	symbol.tickerMu.Lock()
	defer symbol.tickerMu.Unlock()

	insertAt := len(symbol.Tickers)

	for index, existing := range symbol.Tickers {
		if ticker.Timestamp.Before(existing.Timestamp) {
			insertAt = index
			break
		}
	}

	symbol.Tickers = append(symbol.Tickers, kraken.TickerData{})
	copy(symbol.Tickers[insertAt+1:], symbol.Tickers[insertAt:])
	symbol.Tickers[insertAt] = ticker
}

func (symbol *Symbol) AppendTrade(trade kraken.TradeData) {
	symbol.tradeMu.Lock()
	defer symbol.tradeMu.Unlock()

	insertAt := len(symbol.Trades)

	for index, existing := range symbol.Trades {
		if tradeBefore(trade, existing) {
			insertAt = index
			break
		}
	}

	symbol.Trades = append(symbol.Trades, kraken.TradeData{})
	copy(symbol.Trades[insertAt+1:], symbol.Trades[insertAt:])
	symbol.Trades[insertAt] = trade
}

func (symbol *Symbol) MarketTickers(source SourceType) []kraken.TickerData {
	symbol.tickerMu.Lock()
	defer symbol.tickerMu.Unlock()

	if symbol.tickerCursors == nil {
		symbol.tickerCursors = &sync.Map{}
	}

	cursor := 0
	stored, found := symbol.tickerCursors.Load(source)

	if found {
		cursor = stored.(int)
	}

	if cursor >= len(symbol.Tickers) {
		return nil
	}

	rows := slices.Clone(symbol.Tickers[cursor:])
	symbol.tickerCursors.Store(source, len(symbol.Tickers))

	return rows
}

func (symbol *Symbol) MarketTrades(source SourceType) []kraken.TradeData {
	symbol.tradeMu.Lock()
	defer symbol.tradeMu.Unlock()

	if symbol.tradeCursors == nil {
		symbol.tradeCursors = &sync.Map{}
	}

	cursor := 0
	stored, found := symbol.tradeCursors.Load(source)

	if found {
		cursor = stored.(int)
	}

	if cursor >= len(symbol.Trades) {
		return nil
	}

	rows := slices.Clone(symbol.Trades[cursor:])
	symbol.tradeCursors.Store(source, len(symbol.Trades))

	return rows
}

func (symbol *Symbol) TickersSnapshot() []kraken.TickerData {
	symbol.tickerMu.RLock()
	defer symbol.tickerMu.RUnlock()

	return slices.Clone(symbol.Tickers)
}

func (symbol *Symbol) TradesSnapshot() []kraken.TradeData {
	symbol.tradeMu.RLock()
	defer symbol.tradeMu.RUnlock()

	return slices.Clone(symbol.Trades)
}

func (symbol *Symbol) LatestTicker() (kraken.TickerData, bool) {
	symbol.tickerMu.RLock()
	defer symbol.tickerMu.RUnlock()

	if len(symbol.Tickers) == 0 {
		return kraken.TickerData{}, false
	}

	return symbol.Tickers[len(symbol.Tickers)-1], true
}

func tradeBefore(left kraken.TradeData, right kraken.TradeData) bool {
	if left.Timestamp.Equal(right.Timestamp) {
		if left.Symbol == right.Symbol {
			return left.TradeID < right.TradeID
		}

		return left.Symbol < right.Symbol
	}

	return left.Timestamp.Before(right.Timestamp)
}

func (symbol *Symbol) Reset() {
	symbol.measurementMu.Lock()
	defer symbol.measurementMu.Unlock()

	symbol.Status = READY
	symbol.Measurements = symbol.Measurements[:0]
	symbol.Decisions.Clear()
	symbol.Graphs.Clear()
	symbol.Categories.Clear()
	symbol.Phase.Clear()
	symbol.Cognition.Clear()
	symbol.Resonance.Clear()
	symbol.Causal.Clear()
}

/*
AddMeasurement retains the latest measurement for one source and peer and
reports whether that identity actually changed.
*/
func (symbol *Symbol) AddMeasurement(measurement *Measurement) bool {
	if symbol == nil || measurement == nil {
		return false
	}

	symbol.measurementMu.Lock()
	defer symbol.measurementMu.Unlock()

	for index, existing := range symbol.Measurements {
		if existing.Source == measurement.Source && existing.Peer == measurement.Peer {
			if existing.ID == measurement.ID {
				return false
			}

			symbol.Measurements[index] = measurement
			symbol.measurementRevision++
			return true
		}
	}

	symbol.Measurements = append(symbol.Measurements, measurement)
	symbol.measurementRevision++
	return true
}

/*
MeasurementsSnapshot returns an immutable view of the accumulated current evidence.
*/
func (symbol *Symbol) MeasurementsSnapshot() []*Measurement {
	if symbol == nil {
		return nil
	}

	symbol.measurementMu.RLock()
	defer symbol.measurementMu.RUnlock()

	return slices.Clone(symbol.Measurements)
}

/*
MeasurementState returns one immutable evidence cut and its monotonic revision.
*/
func (symbol *Symbol) MeasurementState() ([]*Measurement, uint64, bool) {
	symbol.measurementMu.RLock()
	defer symbol.measurementMu.RUnlock()

	return slices.Clone(symbol.Measurements), symbol.measurementRevision,
		len(symbol.Measurements) > 0
}

/*
MarshalJSON materializes every concurrent symbol state map for durable thesis
checkpoints.
*/
func (symbol *Symbol) MarshalJSON() ([]byte, error) {
	if symbol == nil {
		return []byte("null"), nil
	}

	measurements, revision, _ := symbol.MeasurementState()
	tickers := symbol.TickersSnapshot()
	trades := symbol.TradesSnapshot()

	return sonic.Marshal(struct {
		Symbol              string         `json:"symbol"`
		Status              Status         `json:"status"`
		MeasurementRevision uint64         `json:"measurementRevision"`
		Measurements        []*Measurement `json:"measurements"`
		Tickers             []kraken.TickerData
		Trades              []kraken.TradeData
		Decisions           map[string]any `json:"decisions"`
		Graphs              map[string]any `json:"graphs"`
		Categories          map[string]any `json:"categories"`
		Phase               map[string]any `json:"phase"`
		Cognition           map[string]any `json:"cognition"`
		Resonance           map[string]any `json:"resonance"`
		Causal              map[string]any `json:"causal"`
	}{
		Symbol:              symbol.Symbol,
		Status:              symbol.Status,
		MeasurementRevision: revision,
		Measurements:        measurements,
		Tickers:             tickers,
		Trades:              trades,
		Decisions:           syncMapState(symbol.Decisions),
		Graphs:              syncMapState(symbol.Graphs),
		Categories:          syncMapState(symbol.Categories),
		Phase:               syncMapState(symbol.Phase),
		Cognition:           syncMapState(symbol.Cognition),
		Resonance:           syncMapState(symbol.Resonance),
		Causal:              syncMapState(symbol.Causal),
	})
}

func syncMapState(values *sync.Map) map[string]any {
	state := make(map[string]any)

	if values == nil {
		return state
	}

	values.Range(func(key, value any) bool {
		if checkpointer, ok := value.(interface{ CheckpointState() any }); ok {
			value = checkpointer.CheckpointState()
		}

		state[fmt.Sprint(key)] = value
		return true
	})

	return state
}
